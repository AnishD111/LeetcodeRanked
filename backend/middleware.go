package main

import (
	"context"
	"net/http"
)

type contextKey string

const contextKeyUser contextKey = "authenticated_user"

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required: no session cookie")
			return
		}

		claims, err := ValidateToken(cookie.Value)
		if err != nil {
			clearAuthCookie(w)
			writeError(w, http.StatusUnauthorized, "session expired or invalid — please log in again")
			return
		}

		user, err := GetUserByID(claims.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "user account not found")
			return
		}

		ctx := context.WithValue(r.Context(), contextKeyUser, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

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

type authError struct {
	msg string
}

func (e *authError) Error() string {
	return "auth error: " + e.msg
}
