package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

const TokenDuration = 24 * time.Hour

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("HashPassword: %w", err)
	}
	return string(bytes), nil
}

func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

type AppClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateToken(user UserRecord) (string, error) {
	claims := AppClaims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "leetcoderanked",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(Cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("GenerateToken: %w", err)
	}
	return signed, nil
}

func ValidateToken(tokenString string) (*AppClaims, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
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

func GenerateOAuthState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var GoogleOAuthConfig *oauth2.Config
var GitHubOAuthConfig *oauth2.Config

func InitOAuth() {
	GoogleOAuthConfig = &oauth2.Config{
		ClientID:     Cfg.GoogleClientID,
		ClientSecret: Cfg.GoogleClientSecret,
		RedirectURL:  "http://localhost:" + Cfg.Port + "/api/auth/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}

	GitHubOAuthConfig = &oauth2.Config{
		ClientID:     Cfg.GitHubClientID,
		ClientSecret: Cfg.GitHubClientSecret,
		RedirectURL:  "http://localhost:" + Cfg.Port + "/api/auth/github/callback",
		Scopes:       []string{"user:email"},
		Endpoint:     github.Endpoint,
	}
}
