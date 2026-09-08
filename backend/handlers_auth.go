package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

// ─────────────────────────────────────────────────────────────────────────────
// HELPER UTILITIES
// ─────────────────────────────────────────────────────────────────────────────

// writeJSON is a small helper that sets the Content-Type header and encodes
// any Go value as a JSON response body. This avoids repetitive boilerplate
// across all our handler functions.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// writeError is a helper for returning structured JSON error responses.
// Standardizing error format means the frontend can always parse it the same way.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// setAuthCookie attaches the JWT as an HttpOnly cookie on the HTTP response.
//
// Cookie security flags explained:
//   - HttpOnly: true   → The cookie is inaccessible to JavaScript. Even if an
//                        attacker injects malicious JS into our page (XSS),
//                        they cannot read or steal the token.
//   - SameSite: Lax    → The browser only sends this cookie on same-site
//                        requests, plus top-level navigations from external sites
//                        (like OAuth redirects). Protects against CSRF attacks.
//   - Secure: false    → In production this should be TRUE (only send over HTTPS).
//                        We leave it false for localhost development.
//   - Path: "/"        → The cookie is valid for all routes on our domain.
//   - Expires          → When the browser should automatically discard the cookie.
func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		Expires:  time.Now().Add(TokenDuration),
		// Secure: true, ← enable this when deploying behind HTTPS
	})
}

// clearAuthCookie removes the session cookie by setting an expired one.
// The browser deletes any cookie whose Expires is in the past.
func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		Expires:  time.Unix(0, 0), // Epoch time = already expired → browser deletes it
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// REGISTRATION HANDLER
// POST /api/auth/register
// ─────────────────────────────────────────────────────────────────────────────

// RegisterRequest defines the shape of the JSON body we expect from the client.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleRegister creates a new user account with an email and hashed password.
//
// Step-by-step flow:
//  1. Parse and validate the JSON request body.
//  2. Hash the password with bcrypt (never store raw passwords).
//  3. Insert the new user into the database.
//  4. Generate a JWT and set it as an HttpOnly cookie.
//  5. Return the new user's public profile data.
func handleRegister(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests.
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 1. Parse JSON body
	// io.LimitReader limits the body to 1MB to prevent memory exhaustion from
	// maliciously large request bodies (a basic form of DOS protection).
	var req RegisterRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	defer r.Body.Close()

	// Validate required fields
	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username, email, and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// 2. Hash the password before touching the database
	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process password")
		return
	}

	// 3. Insert into database
	// If the email or username already exists, PostgreSQL will return a
	// unique constraint violation error. We surface this as a 409 Conflict.
	user, err := CreateUserWithPassword(req.Username, req.Email, hash)
	if err != nil {
		// A unique constraint violation means the email/username is taken.
		writeError(w, http.StatusConflict, "username or email already exists")
		return
	}

	// 4. Issue JWT and set HttpOnly cookie
	token, err := GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	setAuthCookie(w, token)

	// 5. Return public profile — never include password_hash in responses!
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"elo_rating": user.EloRating,
		"rank_tier":  user.RankTier,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// LOGIN HANDLER
// POST /api/auth/login
// ─────────────────────────────────────────────────────────────────────────────

// LoginRequest defines the JSON body for the login endpoint.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleLogin authenticates an existing user with email and password.
//
// A critical security principle here is "timing-safe" error messages.
// We return the same generic error whether the email doesn't exist OR the
// password is wrong. This prevents "user enumeration" — if we returned
// "email not found", an attacker could probe which emails have accounts.
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	defer r.Body.Close()

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Fetch user by email.
	// pgx.ErrNoRows means no user with that email exists.
	user, err := GetUserByEmail(req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Use the same error message as wrong-password to prevent enumeration.
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Check if this account was created via OAuth (no password set).
	if user.PasswordHash == "" {
		writeError(w, http.StatusBadRequest, "this account uses social login — please use Google or GitHub")
		return
	}

	// Verify the provided password against the stored bcrypt hash.
	if !CheckPasswordHash(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Password matches — issue JWT and set cookie.
	token, err := GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	setAuthCookie(w, token)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"elo_rating": user.EloRating,
		"rank_tier":  user.RankTier,
		"wins":       user.Wins,
		"losses":     user.Losses,
		"avatar_url": user.AvatarURL,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// LOGOUT HANDLER
// POST /api/auth/logout
// ─────────────────────────────────────────────────────────────────────────────

// handleLogout clears the session cookie, effectively ending the user's session.
//
// NOTE: JWTs are stateless — once issued, they remain valid until they expire,
// even after "logout". Clearing the cookie means the browser no longer sends
// the token, so the user can't make authenticated requests. However, if someone
// already stole the token, they could still use it until expiry (24 hours).
// Production apps mitigate this with a "token revocation list" (Redis/DB blacklist).
// That's out of scope for this milestone, but worth knowing.
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	clearAuthCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

// ─────────────────────────────────────────────────────────────────────────────
// "ME" HANDLER
// GET /api/auth/me
// ─────────────────────────────────────────────────────────────────────────────

// handleMe returns the currently authenticated user's profile.
// This endpoint is protected by our auth middleware — by the time this handler
// runs, the middleware has already validated the JWT and attached the user ID
// to the request context. We just need to extract and use it.
func handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract user from context (set by the auth middleware).
	user, ok := r.Context().Value(contextKeyUser).(UserRecord)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"elo_rating": user.EloRating,
		"rank_tier":  user.RankTier,
		"wins":       user.Wins,
		"losses":     user.Losses,
		"avatar_url": user.AvatarURL,
	})
}

// handleGetEloHistory returns the Elo history log for the currently authenticated user.
func handleGetEloHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, ok := r.Context().Value(contextKeyUser).(UserRecord)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	history, err := GetEloHistoryByUserID(user.ID)
	if err != nil {
		log.Printf("ERROR handleGetEloHistory: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve Elo history")
		return
	}

	if history == nil {
		history = []EloHistoryEntry{}
	}

	writeJSON(w, http.StatusOK, history)
}


// ─────────────────────────────────────────────────────────────────────────────
// GOOGLE OAUTH HANDLERS
// ─────────────────────────────────────────────────────────────────────────────

// handleGoogleLogin redirects the user to Google's OAuth consent screen.
// This is Step 1 of the OAuth flow.
//
// The "state" parameter is a random string we generate and check later.
// It prevents CSRF on the OAuth callback — without it, an attacker could trick
// our server into processing their OAuth code as if it were the user's.
// (In production, this should be a random token stored in a short-lived cookie;
// for simplicity here we use a static string — good enough for development.)
func handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	cfg := GoogleOAuthConfig()
	// AuthCodeURL builds the full Google authorization URL with all our params.
	url := cfg.AuthCodeURL("lcr-state-google", oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// handleGoogleCallback receives the redirect from Google after the user consents.
// This is Steps 3-6 of the OAuth flow.
func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	// Verify the state parameter matches what we sent (CSRF protection).
	if r.URL.Query().Get("state") != "lcr-state-google" {
		writeError(w, http.StatusBadRequest, "invalid OAuth state")
		return
	}

	// Get the authorization code from the URL query string (Step 3).
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing authorization code")
		return
	}

	// Exchange the code for an access token (Step 4).
	cfg := GoogleOAuthConfig()
	oauthToken, err := exchangeOAuthToken(cfg, code)
	if err != nil {
		log.Printf("ERROR handleGoogleCallback exchangeOAuthToken: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to exchange token with Google")
		return
	}


	// Use the access token to fetch the user's Google profile (Step 5).
	googleUser, err := fetchGoogleUser(oauthToken.AccessToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch Google user info")
		return
	}

	// Sanitise Google's display name into a valid username.
	// Google names can contain spaces and mixed case (e.g. "Anish Patel").
	// We strip spaces and lowercase it, then fall back to email-prefix if empty.
	username := sanitiseUsername(googleUser.Name)
	if username == "" {
		username = strings.Split(googleUser.Email, "@")[0]
	}

	// Upsert the user in our database (Step 6).
	user, err := GetOrCreateOAuthUser("google", googleUser.ID, username, googleUser.Email, googleUser.Picture)
	if err != nil {
		// If username is already taken, retry with a unique suffix
		if isUniqueViolation(err, "username") {
			username = username + "_" + googleUser.ID[:6]
			user, err = GetOrCreateOAuthUser("google", googleUser.ID, username, googleUser.Email, googleUser.Picture)
		}
		if err != nil {
			log.Printf("ERROR handleGoogleCallback GetOrCreateOAuthUser: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to create user session")
			return
		}
	}

	// Issue our own JWT and set the HttpOnly cookie.
	token, err := GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate session token")
		return
	}
	setAuthCookie(w, token)

	// Redirect the user back to the frontend app (they're now logged in).
	http.Redirect(w, r, Cfg.FrontendURL+"/game.html", http.StatusTemporaryRedirect)
}

// GoogleUserInfo holds the relevant fields from Google's UserInfo API response.
type GoogleUserInfo struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// fetchGoogleUser calls Google's userinfo endpoint with the provided access token.
func fetchGoogleUser(accessToken string) (*GoogleUserInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// GITHUB OAUTH HANDLERS
// ─────────────────────────────────────────────────────────────────────────────

// handleGitHubLogin redirects the user to GitHub's OAuth consent screen.
func handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	cfg := GitHubOAuthConfig()
	url := cfg.AuthCodeURL("lcr-state-github", oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// handleGitHubCallback handles the redirect from GitHub after user consent.
func handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("state") != "lcr-state-github" {
		writeError(w, http.StatusBadRequest, "invalid OAuth state")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing authorization code")
		return
	}

	cfg := GitHubOAuthConfig()
	oauthToken, err := exchangeOAuthToken(cfg, code)
	if err != nil {
		log.Printf("ERROR handleGitHubCallback exchangeOAuthToken: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to exchange token with GitHub")
		return
	}

	githubUser, err := fetchGitHubUser(oauthToken.AccessToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch GitHub user info")
		return
	}

	// GitHub users may not have a public email set — use "githubid@users.noreply.github.com"
	// as a fallback (GitHub's standard no-reply pattern).
	email := githubUser.Email
	if email == "" {
		email = githubUser.Login + "@users.noreply.github.com"
	}

	username := sanitiseUsername(githubUser.Login)
	if username == "" {
		username = "github_user_" + githubUser.ID[:6]
	}

	user, err := GetOrCreateOAuthUser("github", githubUser.ID, username, email, githubUser.AvatarURL)
	if err != nil {
		// If username is already taken, retry with a unique suffix
		if isUniqueViolation(err, "username") {
			username = username + "_" + githubUser.ID[:6]
			user, err = GetOrCreateOAuthUser("github", githubUser.ID, username, email, githubUser.AvatarURL)
		}
		if err != nil {
			log.Printf("ERROR handleGitHubCallback GetOrCreateOAuthUser: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to create user session")
			return
		}
	}

	token, err := GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate session token")
		return
	}
	setAuthCookie(w, token)

	http.Redirect(w, r, Cfg.FrontendURL+"/game.html", http.StatusTemporaryRedirect)

}

// GitHubUserInfo holds the relevant fields from GitHub's /user API response.
// Note: GitHub uses a numeric ID and "login" for username (not "name").
type GitHubUserInfo struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// fetchGitHubUser calls the GitHub API's /user endpoint.
// GitHub requires the "Accept: application/vnd.github+json" header.
func fetchGitHubUser(accessToken string) (*GitHubUserInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// GitHub returns the user's ID as an integer in JSON.
	// We decode into a temporary struct with int, then convert to string.
	var raw struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	return &GitHubUserInfo{
		ID:        fmt.Sprintf("%d", raw.ID),
		Login:     raw.Login,
		Email:     raw.Email,
		AvatarURL: raw.AvatarURL,
	}, nil
}

// sanitiseUsername converts a display name (e.g. "Anish Patel") into a
// safe username (e.g. "anishpatel") by lowercasing and stripping non-alphanumeric characters.
func sanitiseUsername(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isUniqueViolation checks whether a PostgreSQL error is a unique constraint
// violation (error code 23505) mentioning a specific column name.
func isUniqueViolation(err error, column string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") && strings.Contains(msg, column)
}
