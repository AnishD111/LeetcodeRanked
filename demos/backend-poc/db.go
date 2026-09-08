package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

// Global database connection pointer
var DB *pgx.Conn

// InitDB establishes the connection and sets up tables/seed data
func InitDB(password string) {
	// Construct the connection string. Replace 'localhost' with 'db' if running inside a Docker network later.
	connStr := fmt.Sprintf("postgres://postgres1:%s@localhost:5432/leetranked?sslmode=disable", password)

	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Attempt Connection
	DB, err = pgx.Connect(ctx, connStr)
	if err != nil {
		log.Fatalf("❌ Unable to connect to database: %v\nCheck if your container is running and your password is correct.", err)
	}

	fmt.Println("🔌 Successfully connected to PostgreSQL!")

	// 2. Initialize Tables (Migration)
	createTables()

	// 3. Populate initial questions (Seed)
	seedProblems()
}

func createTables() {
	ctx := context.Background()

	// SQL statements to set up our 4 core tables
	query := `
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		username VARCHAR(50) UNIQUE NOT NULL,
		elo_rating INT DEFAULT 1000 NOT NULL,
		rank_tier VARCHAR(20) DEFAULT 'Bronze' NOT NULL,
		wins INT DEFAULT 0 NOT NULL,
		losses INT DEFAULT 0 NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_users_elo ON users(elo_rating);

	CREATE TABLE IF NOT EXISTS problems (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		title VARCHAR(255) UNIQUE NOT NULL,
		description TEXT NOT NULL,
		difficulty VARCHAR(10) CHECK (difficulty IN ('Easy', 'Medium', 'Hard')) NOT NULL,
		time_limit_ms INT DEFAULT 2000 NOT NULL,
		memory_limit_mb INT DEFAULT 128 NOT NULL,
		class_name VARCHAR(100) NOT NULL,
		method_name VARCHAR(100) NOT NULL,
		starter_templates JSONB NOT NULL, 
		sample_inputs JSONB NOT NULL,   
		sample_outputs JSONB NOT NULL,  
		secret_inputs JSONB NOT NULL,   
		secret_outputs JSONB NOT NULL   
	);

	CREATE TABLE IF NOT EXISTS matches (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		player_1_id UUID REFERENCES users(id) ON DELETE SET NULL,
		player_2_id UUID REFERENCES users(id) ON DELETE SET NULL,
		problem_id UUID REFERENCES problems(id) ON DELETE RESTRICT,
		status VARCHAR(20) CHECK (status IN ('active', 'completed', 'tied')) DEFAULT 'active' NOT NULL,
		winner_id UUID REFERENCES users(id) ON DELETE SET NULL,
		started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		ended_at TIMESTAMP WITH TIME ZONE
	);

	CREATE INDEX IF NOT EXISTS idx_matches_status ON matches(status);

	CREATE TABLE IF NOT EXISTS submissions (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		match_id UUID REFERENCES matches(id) ON DELETE CASCADE NOT NULL,
		user_id UUID REFERENCES users(id) ON DELETE CASCADE NOT NULL,
		code_content TEXT NOT NULL,
		language VARCHAR(20) NOT NULL,
		cases_passed INT NOT NULL,
		total_cases INT NOT NULL,
		status VARCHAR(20) CHECK (status IN ('Accepted', 'Wrong Answer', 'TLE', 'Runtime Error')) NOT NULL,
		submitted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := DB.Exec(ctx, query)
	if err != nil {
		log.Fatalf("❌ Failed to create database tables: %v", err)
	}
	fmt.Println("🏢 Database tables verified/created successfully!")
}
func seedProblems() {
	ctx := context.Background()

	// Seed Two Sum
	twoSumQuery := `
	INSERT INTO problems (
		title, description, difficulty, time_limit_ms, memory_limit_mb, class_name, method_name, starter_templates, sample_inputs, sample_outputs, secret_inputs, secret_outputs
	) VALUES (
		'Two Sum',
		'Given an array of integers ` + "`nums`" + ` and an integer ` + "`target`" + `, return indices of the two numbers such that they add up to ` + "`target`" + `.',
		'Easy',
		2000,
		128,
		'Solution',
		'twoSum',
		'{"python": "class Solution(object):\n    def twoSum(self, nums, target):\n        # Your code here\n        pass"}',
		'[ "[[2, 7, 11, 15], 9]", "[[3, 2, 4], 6]" ]',
		'[ "[0, 1]", "[1, 2]" ]',
		'[ "[[3, 3], 6]" ]',
		'[ "[0, 1]" ]'
	) ON CONFLICT (title) DO NOTHING;`

	// Seed Valid Palindrome (UPDATED FOR CORRECT STRING ESCAPING)
	palindromeQuery := `
	INSERT INTO problems (
		title, description, difficulty, time_limit_ms, memory_limit_mb, class_name, method_name, starter_templates, sample_inputs, sample_outputs, secret_inputs, secret_outputs
	) VALUES (
		'Valid Palindrome',
		'Given a string ` + "`s`" + `, return ` + "`true`" + ` if it is a palindrome, and ` + "`false`" + ` otherwise.',
		'Easy',
		2000,
		128,
		'Solution',
		'isPalindrome',
		'{"python": "class Solution(object):\n    def isPalindrome(self, s):\n        # Your code here\n        pass"}',
		'[ "[\"racecar\"]", "[\"hello\"]" ]',
		'[ "true", "false" ]',
		'[ "[\"amanaplanacanalpanama\"]" ]',
		'[ "true" ]'
	) ON CONFLICT (title) DO NOTHING;`

	_, err := DB.Exec(ctx, twoSumQuery)
	if err != nil {
		log.Printf("⚠️ Warning: Failed to seed Two Sum: %v", err)
	}

	_, err = DB.Exec(ctx, palindromeQuery)
	if err != nil {
		log.Printf("⚠️ Warning: Failed to seed Valid Palindrome: %v", err)
	}

	fmt.Println("🌱 Seeding check complete (seeded questions if they didn't exist).")
}

type ProblemPayload struct {
	ID               string        `json:"id"` // Database UUID
	Title            string        `json:"title"`
	Description      string        `json:"description"`
	ClassName        string        `json:"class_name"`
	MethodName       string        `json:"method_name"`
	StarterTemplates string        `json:"starter_templates"`
	SampleInputs     []string      `json:"sample_inputs"`  // Public examples for UI
	SampleOutputs    []string      `json:"sample_outputs"` // Public outputs for UI
	JudgeConfig      ProblemConfig `json:"-"`              // Secret config for sandbox execution
}

// FetchRandomProblem pulls a random question and separates sample cases from secret cases
// FetchRandomProblem pulls a question and combines public + secret cases for the judge
func FetchRandomProblem() (ProblemPayload, error) {
	ctx := context.Background()

	var payload ProblemPayload
	var sampleInputsRaw string
	var sampleOutputsRaw string
	var secretInputsRaw string
	var secretOutputsRaw string

	query := `
		SELECT 
			id::text, 
			title, 
			description, 
			class_name, 
			method_name, 
			starter_templates::text,
			sample_inputs::text, 
			sample_outputs::text,
			secret_inputs::text, 
			secret_outputs::text 
		FROM problems 
		ORDER BY RANDOM() 
		LIMIT 1;
	`

	err := DB.QueryRow(ctx, query).Scan(
		&payload.ID,
		&payload.Title,
		&payload.Description,
		&payload.ClassName,
		&payload.MethodName,
		&payload.StarterTemplates,
		&sampleInputsRaw,
		&sampleOutputsRaw,
		&secretInputsRaw,
		&secretOutputsRaw,
	)
	if err != nil {
		return ProblemPayload{}, err
	}

	// 1. Unmarshal Sample Cases (for Frontend display)
	if err := json.Unmarshal([]byte(sampleInputsRaw), &payload.SampleInputs); err != nil {
		return ProblemPayload{}, fmt.Errorf("failed parsing sample inputs: %w", err)
	}
	if err := json.Unmarshal([]byte(sampleOutputsRaw), &payload.SampleOutputs); err != nil {
		return ProblemPayload{}, fmt.Errorf("failed parsing sample outputs: %w", err)
	}

	// Unmarshal Secret Cases
	var secretInputs []string
	var secretOutputs []string
	if err := json.Unmarshal([]byte(secretInputsRaw), &secretInputs); err != nil {
		return ProblemPayload{}, fmt.Errorf("failed parsing secret inputs: %w", err)
	}
	if err := json.Unmarshal([]byte(secretOutputsRaw), &secretOutputs); err != nil {
		return ProblemPayload{}, fmt.Errorf("failed parsing secret outputs: %w", err)
	}

	// 2. Combine Sample Cases + Secret Cases into JudgeConfig for total evaluation!
	payload.JudgeConfig.ClassName = payload.ClassName
	payload.JudgeConfig.MethodName = payload.MethodName
	payload.JudgeConfig.TestInputs = append(payload.SampleInputs, secretInputs...)
	payload.JudgeConfig.TestOutputs = append(payload.SampleOutputs, secretOutputs...)

	return payload, nil
}

// UserRecord represents a player in PostgreSQL
type UserRecord struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	EloRating int    `json:"elo_rating"`
}

// GetOrCreateUser fetches an existing user or registers a new one at default 1000 ELO
func GetOrCreateUser(username string) (UserRecord, error) {
	ctx := context.Background()
	var user UserRecord

	query := `
		INSERT INTO users (username)
		VALUES ($1)
		ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username
		RETURNING id, username, elo_rating;
	`
	err := DB.QueryRow(ctx, query, username).Scan(&user.ID, &user.Username, &user.EloRating)
	return user, err
}

// CreateMatchRecord inserts an active match into SQL
func CreateMatchRecord(player1ID, player2ID, problemID string) (string, error) {
	ctx := context.Background()
	var matchID string

	query := `
		INSERT INTO matches (player_1_id, player_2_id, problem_id, status)
		VALUES ($1, $2, $3, 'active')
		RETURNING id;
	`
	err := DB.QueryRow(ctx, query, player1ID, player2ID, problemID).Scan(&matchID)
	return matchID, err
}

// RecordSubmission logs every code attempt
func RecordSubmission(matchID, userID, code, status string, passed, total int) {
	ctx := context.Background()
	query := `
		INSERT INTO submissions (match_id, user_id, code_content, language, cases_passed, total_cases, status)
		VALUES ($1, $2, $3, 'python', $4, $5, $6);
	`
	_, err := DB.Exec(ctx, query, matchID, userID, code, passed, total, status)
	if err != nil {
		log.Printf("⚠️ Failed to record submission: %v\n", err)
	}
}

// CompleteMatch updates match outcome and executes Elo updates atomically
func CompleteMatch(matchID, winnerID, loserID string, newWinnerElo, newLoserElo int) error {
	ctx := context.Background()

	// 1. Mark match completed
	matchQuery := `
		UPDATE matches 
		SET status = 'completed', winner_id = $1, ended_at = CURRENT_TIMESTAMP
		WHERE id = $2;
	`
	_, err := DB.Exec(ctx, matchQuery, winnerID, matchID)
	if err != nil {
		return err
	}

	// 2. Update Winner stats
	winnerQuery := `
		UPDATE users 
		SET elo_rating = $1, wins = wins + 1 
		WHERE id = $2;
	`
	_, err = DB.Exec(ctx, winnerQuery, newWinnerElo, winnerID)
	if err != nil {
		return err
	}

	// 3. Update Loser stats
	loserQuery := `
		UPDATE users 
		SET elo_rating = $1, losses = losses + 1 
		WHERE id = $2;
	`
	_, err = DB.Exec(ctx, loserQuery, newLoserElo, loserID)
	return err
}
