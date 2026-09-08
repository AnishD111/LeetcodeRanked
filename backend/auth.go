package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// ═════════════════════════════════════════════════════════════════════════════
// SECTION 1: PASSWORD HASHING (bcrypt)
//
// WHY CAN'T WE JUST STORE PASSWORDS AS PLAIN TEXT?
// If our database is ever leaked (it happens to companies all the time),
// plain-text passwords give attackers immediate access to every user's account
// — and since people reuse passwords, they'd get into their Gmail, bank, etc.
//
// WHY BCRYPT INSTEAD OF SHA-256 OR MD5?
// SHA-256 and MD5 are general-purpose hash functions designed to be FAST.
// That's bad for passwords — an attacker with a GPU can compute billions of
// SHA-256 hashes per second, brute-forcing common passwords trivially.
//
// bcrypt is intentionally SLOW (configurable via "cost factor"). At cost=12:
//   - Hashing one password takes ~300ms on modern hardware.
//   - For the attacker, brute-forcing 1 billion passwords takes ~10,000 years.
//   - For the legitimate user logging in, 300ms is imperceptible.
//
// bcrypt also automatically generates and embeds a random "salt" into the hash.
// A salt is random bytes added to the password before hashing, so two users
// with the same password produce completely different hash outputs — preventing
// "rainbow table" attacks (precomputed hash dictionaries).
// ═════════════════════════════════════════════════════════════════════════════

// HashPassword takes a raw plaintext password and returns a bcrypt hash string.
//
// The cost factor of 12 is a widely accepted balance between security and speed:
//   - Cost 10: ~100ms (borderline acceptable)
//   - Cost 12: ~300ms (recommended default)
//   - Cost 14: ~1000ms (high-security applications, e.g. financial services)
//
// The returned string looks like: $2a$12$<22 chars salt><31 chars hash>
// That entire string is stored in the database — bcrypt.CompareHashAndPassword
// extracts the salt automatically when comparing later.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("HashPassword: %w", err)
	}
	return string(bytes), nil
}

// CheckPasswordHash compares a plaintext password against a stored bcrypt hash.
//
// IMPORTANT: We never "decrypt" the password — bcrypt is a one-way function.
// Instead, bcrypt re-hashes the candidate password with the same salt that's
// embedded in the stored hash, then compares the results.
//
// Returns nil if the password matches, or an error if it doesn't.
// We return a generic boolean rather than exposing the specific error, so
// callers can't infer information from error messages (a security consideration).
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ═════════════════════════════════════════════════════════════════════════════
// SECTION 2: JSON WEB TOKENS (JWT)
//
// WHAT IS A JWT?
// A JWT is a compact, self-contained string that encodes identity claims.
// It has three parts separated by dots:
//
//   HEADER.PAYLOAD.SIGNATURE
//
// Example token:
//   eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoiYWJjMTIzIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV
//
// 1. HEADER:    Base64-encoded JSON describing the algorithm (e.g. HS256).
// 2. PAYLOAD:   Base64-encoded JSON with "claims" — data we want to store.
//               Claims can be: user ID, username, expiry time, etc.
//               IMPORTANT: The payload is NOT encrypted — it's just encoded.
//               Anyone can decode it. Never store passwords or secrets in it.
// 3. SIGNATURE: HMAC-SHA256(base64(header) + "." + base64(payload), JWT_SECRET)
//               Only our server knows JWT_SECRET, so only we can produce a
//               valid signature. Tampering with the payload invalidates it.
//
// WHY COOKIES INSTEAD OF LOCALSTORAGE?
// A common mistake is storing JWTs in browser localStorage.
// localStorage is accessible to ANY JavaScript on the page, making it
// vulnerable to XSS (Cross-Site Scripting) attacks.
//
// HttpOnly cookies cannot be read by JavaScript at all — only the browser
// sends them automatically with HTTP requests. This eliminates the XSS vector.
// ═════════════════════════════════════════════════════════════════════════════

// TokenDuration sets how long a JWT session lasts before the user must re-login.
const TokenDuration = 24 * time.Hour

// AppClaims defines what data we embed inside the JWT payload.
//
// We extend jwt.RegisteredClaims which includes standard fields like:
//   - ExpiresAt: The timestamp after which the token is rejected.
//   - IssuedAt:  When the token was created (useful for auditing).
//
// We add our own application-specific fields:
//   - UserID:   The UUID of the authenticated user (primary key in DB).
//   - Username: Stored for display without needing an extra DB lookup.
type AppClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenerateToken creates and signs a new JWT for an authenticated user.
//
// Flow:
//  1. Create an AppClaims struct populated with the user's data + expiry time.
//  2. Create a new jwt.Token using the HS256 signing algorithm.
//  3. Sign the token using our JWT_SECRET from config — this produces the
//     final "header.payload.signature" string.
//  4. Return the signed string, which is stored in the user's HttpOnly cookie.
func GenerateToken(user UserRecord) (string, error) {
	claims := AppClaims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			// ExpiresAt tells the JWT library (and any receiver) when to reject this token.
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenDuration)),
			// IssuedAt records when the token was minted, useful for audit logs.
			IssuedAt: jwt.NewNumericDate(time.Now()),
			// Issuer identifies who created the token (our application name).
			Issuer: "leetcoderanked",
		},
	}

	// jwt.NewWithClaims bundles the algorithm + claims into an unsigned token object.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// .SignedString() takes our secret key and computes the HMAC-SHA256 signature,
	// producing the final compact token string.
	signed, err := token.SignedString([]byte(Cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("GenerateToken: %w", err)
	}
	return signed, nil
}

// ValidateToken parses and verifies a JWT string.
//
// Flow:
//  1. Parse the token string — the library base64-decodes header + payload.
//  2. Recompute the signature using our JWT_SECRET and compare it to the token's
//     signature. If they don't match → the token was tampered with → reject.
//  3. Check the ExpiresAt claim — if in the past → the token is expired → reject.
//  4. If all checks pass → return the embedded AppClaims (containing user ID).
//
// The `keyFunc` callback is how we tell the JWT library which secret to use.
// It receives the unverified token (so we can inspect the algorithm) before
// verification. We explicitly reject non-HS256 algorithms to prevent the
// notorious "alg:none" attack where a malicious token claims no signing was done.
func ValidateToken(tokenString string) (*AppClaims, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		// Guard: ensure the algorithm is exactly what we expect.
		// Without this check, an attacker could forge a token with alg="none"
		// and bypass signature verification entirely.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(Cfg.JWTSecret), nil
	}

	claims := &AppClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, keyFunc)
	if err != nil {
		return nil, fmt.Errorf("ValidateToken: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("token is not valid")
	}
	return claims, nil
}

// ═════════════════════════════════════════════════════════════════════════════
// SECTION 3: OAUTH 2.0 CONFIGURATION
//
// HOW DOES OAUTH 2.0 WORK? (The "Authorization Code Flow")
// OAuth lets a user prove their identity using a trusted third party (Google,
// GitHub) without ever sharing their password with us. Here's the dance:
//
//  STEP 1 — Redirect: We send the user to Google's /authorize URL with our
//            CLIENT_ID and a REDIRECT_URI (our callback endpoint).
//
//  STEP 2 — Consent: The user sees Google's consent screen ("Allow LeetCode
//            Ranked to see your email?") and approves.
//
//  STEP 3 — Code: Google redirects back to our REDIRECT_URI with a short-lived
//            one-time "authorization code" in the URL query string.
//
//  STEP 4 — Exchange: Our server-side code takes that code and exchanges it
//            (along with our CLIENT_SECRET) for an "access token" via a
//            server-to-server POST to Google's token endpoint.
//            NOTE: The CLIENT_SECRET never leaves our server — that's why
//            this step happens server-side, not in the browser.
//
//  STEP 5 — Profile: We use the access token to call Google's UserInfo API
//            and get the user's email, name, avatar, and unique Google ID.
//
//  STEP 6 — Session: We upsert the user in our DB and issue our own JWT cookie.
//            From this point, OAuth is no longer involved — our app takes over.
//
// CLIENT_ID: Public identifier for our app (safe to expose in browser).
// CLIENT_SECRET: Private key proving we are who we say we are (server-only).
// ═════════════════════════════════════════════════════════════════════════════

// GoogleOAuthConfig returns the OAuth2 config for Google.
// The scopes define what information we request access to:
//   - "openid"  = standard OpenID Connect (identity confirmation)
//   - "email"   = access to the user's email address
//   - "profile" = access to name and avatar picture
func GoogleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     Cfg.GoogleClientID,
		ClientSecret: Cfg.GoogleClientSecret,
		RedirectURL:  "http://localhost:" + Cfg.Port + "/api/auth/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint, // Pre-configured Google OAuth endpoints
	}
}

// GitHubOAuthConfig returns the OAuth2 config for GitHub.
// The "user:email" scope requests read-access to the user's email addresses.
func GitHubOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     Cfg.GitHubClientID,
		ClientSecret: Cfg.GitHubClientSecret,
		RedirectURL:  "http://localhost:" + Cfg.Port + "/api/auth/github/callback",
		Scopes:       []string{"user:email"},
		Endpoint:     github.Endpoint, // Pre-configured GitHub OAuth endpoints
	}
}

// exchangeOAuthToken exchanges an authorization code for an access token.
// This is Step 4 of the OAuth flow described above.
// The `ctx` carries a timeout so we don't hang if the OAuth provider is slow.
func exchangeOAuthToken(cfg *oauth2.Config, code string) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("OAuth code exchange failed: %w", err)
	}
	return token, nil
}
