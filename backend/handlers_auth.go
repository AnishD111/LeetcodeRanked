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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		Expires:  time.Now().Add(TokenDuration),
	})
}

func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		Expires:  time.Unix(0, 0),
	})
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if len(req.Username) < 3 || len(req.Username) > 30 {
		writeError(w, http.StatusBadRequest, "username must be between 3 and 30 characters")
		return
	}
	if !strings.Contains(req.Email, "@") || !strings.Contains(req.Email, ".") {
		writeError(w, http.StatusBadRequest, "please provide a valid email address")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters long")
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process credentials")
		return
	}

	user, err := CreateUserWithPassword(req.Username, req.Email, hash)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeError(w, http.StatusConflict, "username or email is already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user account")
		return
	}

	token, err := GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate session")
		return
	}

	setAuthCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{
		"message": "account created successfully",
		"user": map[string]any{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"elo_rating": user.EloRating,
			"rank_tier":  user.RankTier,
			"wins":       user.Wins,
			"losses":     user.Losses,
		},
	})
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))

	user, err := GetUserByEmail(email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "login error")
		return
	}

	if user.PasswordHash == "" {
		writeError(w, http.StatusUnauthorized, "this account uses Google or GitHub login")
		return
	}

	if !CheckPassword(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate session")
		return
	}

	setAuthCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "login successful",
		"user": map[string]any{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"elo_rating": user.EloRating,
			"rank_tier":  user.RankTier,
			"wins":       user.Wins,
			"losses":     user.Losses,
		},
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	clearAuthCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(contextKeyUser).(UserRecord)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"avatar_url": user.AvatarURL,
		"elo_rating": user.EloRating,
		"rank_tier":  user.RankTier,
		"wins":       user.Wins,
		"losses":     user.Losses,
	})
}

func handleGetEloHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(contextKeyUser).(UserRecord)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	history, err := GetEloHistoryByUserID(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch Elo history")
		return
	}

	if history == nil {
		history = []EloHistoryEntry{}
	}

	writeJSON(w, http.StatusOK, history)
}

func handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := GenerateOAuthState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize OAuth session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		Expires:  time.Now().Add(10 * time.Minute),
	})

	url := GoogleOAuthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "OAuth state validation failed", http.StatusBadRequest)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "oauth_state",
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
	})

	code := r.URL.Query().Get("code")
	token, err := GoogleOAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	client := GoogleOAuthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		http.Error(w, "Failed to fetch profile", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var gUser struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &gUser); err != nil {
		http.Error(w, "Failed to parse Google user data", http.StatusInternalServerError)
		return
	}

	baseUsername := sanitizeUsername(gUser.Name)
	if baseUsername == "" {
		parts := strings.Split(gUser.Email, "@")
		baseUsername = sanitizeUsername(parts[0])
	}
	if len(baseUsername) > 20 {
		baseUsername = baseUsername[:20]
	}
	username := fmt.Sprintf("%s_%s", baseUsername, gUser.ID[:min(6, len(gUser.ID))])

	user, err := GetOrCreateOAuthUser("google", gUser.ID, username, gUser.Email, gUser.Picture)
	if err != nil {
		log.Printf("Google OAuth DB Error: %v\n", err)
		http.Error(w, "Failed to sync user account: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jwtToken, err := GenerateToken(user)
	if err != nil {
		http.Error(w, "Failed to generate session", http.StatusInternalServerError)
		return
	}

	setAuthCookie(w, jwtToken)
	http.Redirect(w, r, Cfg.FrontendURL, http.StatusTemporaryRedirect)
}

func handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	state, err := GenerateOAuthState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize OAuth session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		Expires:  time.Now().Add(10 * time.Minute),
	})

	url := GitHubOAuthConfig.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "OAuth state validation failed", http.StatusBadRequest)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "oauth_state",
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
	})

	code := r.URL.Query().Get("code")
	token, err := GitHubOAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	client := GitHubOAuthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		http.Error(w, "Failed to fetch profile", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var ghUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(body, &ghUser); err != nil {
		http.Error(w, "Failed to parse profile", http.StatusInternalServerError)
		return
	}

	if ghUser.Email == "" {
		ghUser.Email = fetchGitHubPrimaryEmail(client)
	}

	ghIDStr := fmt.Sprintf("%d", ghUser.ID)
	baseUsername := sanitizeUsername(ghUser.Login)
	if len(baseUsername) > 20 {
		baseUsername = baseUsername[:20]
	}
	username := fmt.Sprintf("%s_%s", baseUsername, ghIDStr[:min(6, len(ghIDStr))])

	user, err := GetOrCreateOAuthUser("github", ghIDStr, username, ghUser.Email, ghUser.AvatarURL)
	if err != nil {
		log.Printf("GitHub OAuth DB Error: %v\n", err)
		http.Error(w, "Failed to sync user account: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jwtToken, err := GenerateToken(user)
	if err != nil {
		http.Error(w, "Failed to generate session", http.StatusInternalServerError)
		return
	}

	setAuthCookie(w, jwtToken)
	http.Redirect(w, r, Cfg.FrontendURL, http.StatusTemporaryRedirect)
}

func fetchGitHubPrimaryEmail(client *http.Client) string {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return ""
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	if len(emails) > 0 {
		return emails[0].Email
	}
	return ""
}

func sanitizeUsername(input string) string {
	var sb strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			sb.WriteRune(r)
		}
	}
	res := sb.String()
	if len(res) == 0 {
		return "user"
	}
	return res
}
