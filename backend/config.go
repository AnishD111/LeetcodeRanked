package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	DatabaseURL        string
	JWTSecret          string
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	FrontendURL        string
}

var Cfg Config

func LoadConfig() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found. Reading from system environment.")
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

	log.Println("Configuration loaded.")
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("FATAL: Required environment variable '%s' is not set.", key)
	}
	return val
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}
