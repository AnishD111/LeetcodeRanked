package main

import (
	"context"
	"net/http"
)

// ═════════════════════════════════════════════════════════════════════════════
// WHAT IS MIDDLEWARE?
//
// In web development, "middleware" is a function that wraps around HTTP handlers.
// It runs BEFORE your actual handler logic, allowing you to:
//   - Inspect or modify the incoming request (e.g. validate a token)
//   - Reject requests early (e.g. return 401 before the handler runs)
//   - Attach data to the request for handlers to use (e.g. the user object)
//   - Run code AFTER the handler responds (e.g. logging)
//
// The pattern in Go looks like this:
//
//   func MyMiddleware(next http.Handler) http.Handler {
//       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//           // Code here runs BEFORE the handler
//           next.ServeHTTP(w, r)   // ← calls the actual handler
//           // Code here runs AFTER the handler
//       })
//   }
//
// You "stack" middleware by wrapping:
//   router.Handle("/api/me", AuthMiddleware(handleMe))
//
// Our auth middleware:
//  1. Reads the "auth_token" cookie from the request.
//  2. Validates the JWT inside it.
//  3. Fetches the full user record from the database.
//  4. Attaches the UserRecord to the request context.
//  5. Calls next.ServeHTTP — passing control to the actual handler.
//  6. If any step fails → returns 401 Unauthorized, handler never runs.
// ═════════════════════════════════════════════════════════════════════════════

// contextKey is a custom unexported type used as a key for request context values.
//
// WHY A CUSTOM TYPE? Go's context.WithValue takes an `any` key and warns against
// using built-in types (string, int) directly, because multiple packages could
// accidentally use the same string key and overwrite each other's values.
// Using a package-private type guarantees our keys are unique to this package.
type contextKey string

const contextKeyUser contextKey = "authenticated_user"

// RequireAuth is the HTTP middleware that protects REST API endpoints.
// Wrap any handler with this to enforce authentication.
//
// Usage in main.go:
//   http.Handle("/api/auth/me", RequireAuth(http.HandlerFunc(handleMe)))
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Step 1: Read the auth_token cookie.
		// r.Cookie("auth_token") returns http.ErrNoCookie if not present.
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			// No cookie = no session = not authenticated.
			writeError(w, http.StatusUnauthorized, "authentication required: no session cookie")
			return
		}

		// Step 2: Validate the JWT.
		// ValidateToken checks the signature, algorithm, and expiry.
		claims, err := ValidateToken(cookie.Value)
		if err != nil {
			// Invalid or expired token — clear the bad cookie and reject.
			clearAuthCookie(w)
			writeError(w, http.StatusUnauthorized, "session expired or invalid — please log in again")
			return
		}

		// Step 3: Fetch the full user from the database using the user ID from the token.
		//
		// WHY FETCH FROM DB INSTEAD OF TRUSTING ONLY THE TOKEN?
		// The JWT contains the user's ID and username at the time it was issued.
		// But what if we needed to ban a user, or their username changed?
		// The token would still carry stale data. By doing a DB lookup here,
		// we get fresh, authoritative data on every request.
		// The tradeoff: one DB query per authenticated request. For most apps
		// this is worth the consistency guarantee.
		user, err := GetUserByID(claims.UserID)
		if err != nil {
			// User was deleted from DB but their token is still valid? Reject.
			writeError(w, http.StatusUnauthorized, "user account not found")
			return
		}

		// Step 4: Attach the UserRecord to the request context.
		// Handlers downstream can retrieve it with:
		//   user := r.Context().Value(contextKeyUser).(UserRecord)
		ctx := context.WithValue(r.Context(), contextKeyUser, user)

		// Step 5: Pass control to the next handler with the enriched context.
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ═════════════════════════════════════════════════════════════════════════════
// WEBSOCKET AUTHENTICATION
//
// WHAT'S DIFFERENT ABOUT AUTHENTICATING A WEBSOCKET?
// HTTP requests are stateless — every request carries its own cookies/headers,
// and our middleware can easily intercept them before the handler runs.
//
// WebSocket connections are different in two key ways:
//
//  1. THE UPGRADE: A WebSocket connection starts as a normal HTTP request (the
//     "handshake"). After the server responds with HTTP 101 Switching Protocols,
//     the connection upgrades to a persistent, full-duplex TCP channel.
//     Our authentication window is ONLY during this initial HTTP handshake.
//     Once upgraded, it's a raw TCP stream — HTTP concepts like cookies no longer
//     apply in the same way.
//
//  2. TIMING: We must authenticate the user BEFORE calling upgrader.Upgrade().
//     If we call Upgrade() first, the HTTP response is already committed —
//     we can't send a 401 anymore. So authentication must happen first.
//
// Our WS auth strategy:
//  - Read the auth_token cookie from the upgrade request (it's a regular HTTP
//    request at this point, so cookies are available normally).
//  - Validate the JWT.
//  - If valid → proceed to upgrade + queue the player.
//  - If invalid → return 401 and abort (never upgrade).
// ═════════════════════════════════════════════════════════════════════════════

// authenticateWSRequest extracts and validates the JWT from a WebSocket upgrade
// request. Returns the authenticated UserRecord, or an error if unauthenticated.
//
// This is a function (not middleware) because WebSocket handling requires the
// gorilla upgrader to be called explicitly — we can't use the standard
// http.Handler wrapping pattern cleanly for WS endpoints.
func authenticateWSRequest(r *http.Request) (UserRecord, error) {
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return UserRecord{}, &authError{"no auth cookie present"}
	}

	claims, err := ValidateToken(cookie.Value)
	if err != nil {
		return UserRecord{}, &authError{"invalid or expired token"}
	}

	user, err := GetUserByID(claims.UserID)
	if err != nil {
		return UserRecord{}, &authError{"user not found"}
	}

	return user, nil
}

// authError is a simple custom error type for authentication failures.
// Wrapping it lets callers distinguish auth errors from other errors (e.g. DB errors).
type authError struct {
	msg string
}

func (e *authError) Error() string {
	return "auth error: " + e.msg
}
