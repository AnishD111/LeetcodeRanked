package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var DBPool *pgxpool.Pool

type UserRecord struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	GoogleID     string
	GitHubID     string
	AvatarURL    string
	EloRating    int
	RankTier     string
	Wins         int
	Losses       int
}

type JudgeConfig struct {
	ClassName  string          `json:"class_name"`
	MethodName string          `json:"method_name"`
	TestCases  []TestCaseEntry `json:"test_cases"`
}

type TestCaseEntry struct {
	Input    json.RawMessage `json:"input"`
	Expected json.RawMessage `json:"expected"`
}

type ProblemRecord struct {
	ID               string
	Title            string
	Difficulty       string
	Description      string
	StarterTemplates map[string]string
	JudgeConfig      JudgeConfig
}

func InitDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	DBPool, err = pgxpool.New(ctx, Cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("❌ Unable to create database pool: %v", err)
	}

	if err = DBPool.Ping(ctx); err != nil {
		log.Fatalf("❌ Unable to ping database: %v", err)
	}

	fmt.Println("🔌 Successfully connected to PostgreSQL (connection pool active)!")

	createTables()
	seedProblems()
}

func createTables() {
	ctx := context.Background()

	query := `
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

	CREATE TABLE IF NOT EXISTS users (
		id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		username      VARCHAR(50)  UNIQUE NOT NULL,
		email         VARCHAR(255) UNIQUE,
		password_hash TEXT,
		google_id     VARCHAR(255) UNIQUE,
		github_id     VARCHAR(255) UNIQUE,
		avatar_url    TEXT,
		elo_rating    INT  DEFAULT 1000 NOT NULL,
		rank_tier     VARCHAR(20) DEFAULT 'Bronze' NOT NULL,
		wins          INT  DEFAULT 0 NOT NULL,
		losses        INT  DEFAULT 0 NOT NULL,
		created_at    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	ALTER TABLE users ADD COLUMN IF NOT EXISTS email         VARCHAR(255);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS google_id     VARCHAR(255);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS github_id     VARCHAR(255);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url    TEXT;

	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email     ON users(email)     WHERE email IS NOT NULL;
	
	DROP INDEX IF EXISTS idx_users_google_id;
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_google_id ON users(google_id);
	
	DROP INDEX IF EXISTS idx_users_github_id;
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_github_id ON users(github_id);
	
	CREATE INDEX        IF NOT EXISTS idx_users_elo       ON users(elo_rating);


	CREATE TABLE IF NOT EXISTS problems (
		id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		title             VARCHAR(255) NOT NULL,
		difficulty        VARCHAR(20)  NOT NULL,
		description       TEXT NOT NULL,
		starter_templates JSONB NOT NULL DEFAULT '{}',
		sample_inputs     JSONB NOT NULL DEFAULT '[]',
		sample_outputs    JSONB NOT NULL DEFAULT '[]',
		secret_inputs     JSONB NOT NULL DEFAULT '[]',
		secret_outputs    JSONB NOT NULL DEFAULT '[]',
		class_name        VARCHAR(100) NOT NULL,
		method_name       VARCHAR(100) NOT NULL,
		created_at        TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS matches (
		id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		player_a_id  UUID REFERENCES users(id),
		player_b_id  UUID REFERENCES users(id),
		problem_id   UUID REFERENCES problems(id),
		winner_id    UUID REFERENCES users(id),
		status       VARCHAR(20) DEFAULT 'active',
		started_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP WITH TIME ZONE
	);

	ALTER TABLE matches ADD COLUMN IF NOT EXISTS player_a_id UUID REFERENCES users(id);
	ALTER TABLE matches ADD COLUMN IF NOT EXISTS player_b_id UUID REFERENCES users(id);
	ALTER TABLE matches ADD COLUMN IF NOT EXISTS winner_id   UUID REFERENCES users(id);
	ALTER TABLE matches ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP WITH TIME ZONE;



	CREATE TABLE IF NOT EXISTS submissions (
		id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		match_id     UUID REFERENCES matches(id),
		user_id      UUID REFERENCES users(id),
		code         TEXT NOT NULL,
		language     VARCHAR(20) DEFAULT 'python',
		passed       BOOLEAN NOT NULL,
		submitted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS elo_history (
		id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
		match_id    UUID REFERENCES matches(id) ON DELETE SET NULL,
		old_elo     INT NOT NULL,
		new_elo     INT NOT NULL,
		recorded_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_elo_history_user ON elo_history(user_id);
	`

	_, err := DBPool.Exec(ctx, query)
	if err != nil {
		log.Fatalf("❌ Failed to create tables: %v", err)
	}
	fmt.Println("📋 Database tables verified/created.")
}

// ─────────────────────────────────────────────────────────────────────────────
// USER CRUD
// ─────────────────────────────────────────────────────────────────────────────

func CreateUserWithPassword(username, email, passwordHash string) (UserRecord, error) {
	ctx := context.Background()

	query := `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, username, COALESCE(email,''), COALESCE(password_hash,''),
		          COALESCE(google_id,''), COALESCE(github_id,''), COALESCE(avatar_url,''),
		          elo_rating, rank_tier, wins, losses
	`

	var u UserRecord
	err := DBPool.QueryRow(ctx, query, username, email, passwordHash).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.GoogleID, &u.GitHubID, &u.AvatarURL,
		&u.EloRating, &u.RankTier, &u.Wins, &u.Losses,
	)
	if err != nil {
		return UserRecord{}, fmt.Errorf("CreateUserWithPassword: %w", err)
	}
	return u, nil
}

func GetUserByEmail(email string) (UserRecord, error) {
	ctx := context.Background()

	query := `
		SELECT id, username, COALESCE(email,''), COALESCE(password_hash,''),
		       COALESCE(google_id,''), COALESCE(github_id,''), COALESCE(avatar_url,''),
		       elo_rating, rank_tier, wins, losses
		FROM users WHERE email = $1
	`

	var u UserRecord
	err := DBPool.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.GoogleID, &u.GitHubID, &u.AvatarURL,
		&u.EloRating, &u.RankTier, &u.Wins, &u.Losses,
	)
	if err != nil {
		return UserRecord{}, err
	}
	return u, nil
}

func GetUserByID(id string) (UserRecord, error) {
	ctx := context.Background()

	query := `
		SELECT id, username, COALESCE(email,''), COALESCE(password_hash,''),
		       COALESCE(google_id,''), COALESCE(github_id,''), COALESCE(avatar_url,''),
		       elo_rating, rank_tier, wins, losses
		FROM users WHERE id = $1
	`

	var u UserRecord
	err := DBPool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.GoogleID, &u.GitHubID, &u.AvatarURL,
		&u.EloRating, &u.RankTier, &u.Wins, &u.Losses,
	)
	if err != nil {
		return UserRecord{}, err
	}
	return u, nil
}

type EloHistoryEntry struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	MatchID    *string   `json:"match_id"`
	OldElo     int       `json:"old_elo"`
	NewElo     int       `json:"new_elo"`
	RecordedAt time.Time `json:"recorded_at"`
}

func GetEloHistoryByUserID(userID string) ([]EloHistoryEntry, error) {
	ctx := context.Background()
	query := `
		SELECT id, user_id, match_id, old_elo, new_elo, recorded_at
		FROM elo_history
		WHERE user_id = $1
		ORDER BY recorded_at DESC
	`
	rows, err := DBPool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []EloHistoryEntry
	for rows.Next() {
		var entry EloHistoryEntry
		err := rows.Scan(&entry.ID, &entry.UserID, &entry.MatchID, &entry.OldElo, &entry.NewElo, &entry.RecordedAt)
		if err != nil {
			return nil, err
		}
		history = append(history, entry)
	}
	return history, nil
}


// GetOrCreateOAuthUser upserts a user identified by their OAuth provider ID.
func GetOrCreateOAuthUser(provider, providerID, username, email, avatarURL string) (UserRecord, error) {
	ctx := context.Background()

	var idColumn string
	switch provider {
	case "google":
		idColumn = "google_id"
	case "github":
		idColumn = "github_id"
	default:
		return UserRecord{}, fmt.Errorf("unknown OAuth provider: %s", provider)
	}

	query := fmt.Sprintf(`
		INSERT INTO users (username, email, %s, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (%s) DO UPDATE
			SET avatar_url = EXCLUDED.avatar_url
		RETURNING id, username, COALESCE(email,''), COALESCE(password_hash,''),
		          COALESCE(google_id,''), COALESCE(github_id,''), COALESCE(avatar_url,''),
		          elo_rating, rank_tier, wins, losses
	`, idColumn, idColumn)

	var u UserRecord
	err := DBPool.QueryRow(ctx, query, username, email, providerID, avatarURL).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.GoogleID, &u.GitHubID, &u.AvatarURL,
		&u.EloRating, &u.RankTier, &u.Wins, &u.Losses,
	)
	if err != nil {
		return UserRecord{}, fmt.Errorf("GetOrCreateOAuthUser (%s): %w", provider, err)
	}
	return u, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MATCH & SUBMISSION
// ─────────────────────────────────────────────────────────────────────────────

func FetchRandomProblem() (ProblemRecord, error) {
	ctx := context.Background()

	query := `
		SELECT id, title, difficulty, description, starter_templates,
		       sample_inputs, sample_outputs, secret_inputs, secret_outputs,
		       class_name, method_name
		FROM problems
		ORDER BY RANDOM()
		LIMIT 1
	`

	var (
		p                                       ProblemRecord
		templatesRaw                            []byte
		sampleInputs, sampleOutputs             []json.RawMessage
		secretInputs, secretOutputs             []json.RawMessage
	)

	err := DBPool.QueryRow(ctx, query).Scan(
		&p.ID, &p.Title, &p.Difficulty, &p.Description, &templatesRaw,
		&sampleInputs, &sampleOutputs,
		&secretInputs, &secretOutputs,
		&p.JudgeConfig.ClassName, &p.JudgeConfig.MethodName,
	)
	if err != nil {
		return ProblemRecord{}, fmt.Errorf("FetchRandomProblem: %w", err)
	}

	if err := json.Unmarshal(templatesRaw, &p.StarterTemplates); err != nil {
		return ProblemRecord{}, fmt.Errorf("FetchRandomProblem template parse: %w", err)
	}

	for i := range sampleInputs {
		p.JudgeConfig.TestCases = append(p.JudgeConfig.TestCases, TestCaseEntry{
			Input: sampleInputs[i], Expected: sampleOutputs[i],
		})
	}
	for i := range secretInputs {
		p.JudgeConfig.TestCases = append(p.JudgeConfig.TestCases, TestCaseEntry{
			Input: secretInputs[i], Expected: secretOutputs[i],
		})
	}

	return p, nil
}

func CreateMatchRecord(playerAID, playerBID, problemID string) (string, error) {
	ctx := context.Background()
	var matchID string
	err := DBPool.QueryRow(ctx,
		`INSERT INTO matches (player_a_id, player_b_id, problem_id)
		 VALUES ($1, $2, $3) RETURNING id`,
		playerAID, playerBID, problemID,
	).Scan(&matchID)
	return matchID, err
}

func RecordSubmission(matchID, userID, code string, passed bool) error {
	ctx := context.Background()
	_, err := DBPool.Exec(ctx,
		`INSERT INTO submissions (match_id, user_id, code, passed)
		 VALUES ($1, $2, $3, $4)`,
		matchID, userID, code, passed,
	)
	return err
}

func CompleteMatch(matchID, winnerID, loserID string) (int, int, int, int, error) {
	ctx := context.Background()

	_, err := DBPool.Exec(ctx,
		`UPDATE matches SET winner_id=$1, status='completed', completed_at=NOW()
		 WHERE id=$2`,
		winnerID, matchID,
	)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return UpdateEloRatings(matchID, winnerID, loserID)
}



// ─────────────────────────────────────────────────────────────────────────────
// SEED DATA
// ─────────────────────────────────────────────────────────────────────────────

func seedProblems() {
	ctx := context.Background()

	var count int
	_ = DBPool.QueryRow(ctx, "SELECT COUNT(*) FROM problems").Scan(&count)
	if count > 0 {
		fmt.Printf("Problems table has %d entries.\n", count)
	} else {
		fmt.Println("Problems table is empty. Run scripts/seed_db.py to populate it.")
	}
}

func scanUser(rows pgx.Rows) (UserRecord, error) {
	return pgx.CollectOneRow(rows, func(row pgx.CollectableRow) (UserRecord, error) {
		var u UserRecord
		return u, row.Scan(
			&u.ID, &u.Username, &u.Email, &u.PasswordHash,
			&u.GoogleID, &u.GitHubID, &u.AvatarURL,
			&u.EloRating, &u.RankTier, &u.Wins, &u.Losses,
		)
	})
}


