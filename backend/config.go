package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all environment-loaded configuration for the application.
// By grouping config into a single struct, any part of the codebase can access
// settings through the global `Cfg` variable without needing to call os.Getenv
// scattered throughout the code.
type Config struct {
	Port string

	// Database connection string (e.g. postgres://user:pass@host/db)
	DatabaseURL string

	// Secret key used to sign and verify JWT tokens.
	// Think of this like a private stamp — only our server knows it.
	JWTSecret string

	// Google OAuth 2.0 credentials (obtained from Google Cloud Console)
	GoogleClientID     string
	GoogleClientSecret string

	// GitHub OAuth 2.0 credentials (obtained from GitHub Developer Settings)
	GitHubClientID     string
	GitHubClientSecret string

	// The base URL of the frontend application — used for CORS and OAuth redirects
	FrontendURL string
}

// Cfg is the global configuration instance.
// It is populated once at startup via LoadConfig() and then read-only.
var Cfg Config

// LoadConfig reads the .env file and then populates the Cfg struct.
// It must be called before any other initialization (DB, routes, etc.)
func LoadConfig() {
	// godotenv.Load() reads the .env file in the current directory and loads
	// each KEY=VALUE pair into the process environment (os.Getenv works after this).
	// If the file is missing, we log a warning but don't crash — production
	// environments typically inject env vars directly, not from a file.
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Warning: No .env file found. Reading from system environment instead.")
	}

	Cfg = Config{
		Port:               getEnvOrDefault("PORT", "8080"),
		DatabaseURL:        mustGetEnv("DB_URL"),
		JWTSecret:          mustGetEnv("JWT_SECRET"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		FrontendURL:        getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
	}

	log.Println("✅ Configuration loaded successfully.")
}

// mustGetEnv retrieves a required environment variable.
// If the variable is not set or is empty, the application crashes immediately.
// This is intentional — missing critical config (like JWT_SECRET or DB_URL)
// means the server cannot run safely, so failing fast is the right behavior.
func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("❌ FATAL: Required environment variable '%s' is not set.", key)
	}
	return val
}

// getEnvOrDefault retrieves an optional environment variable.
// If not set, it falls back to the provided default value.
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}
