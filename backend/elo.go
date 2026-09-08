package main

import (
	"context"
	"fmt"
	"math"
)

// ═════════════════════════════════════════════════════════════════════════════
// THE ELO RATING SYSTEM
//
// Elo was invented by Arpad Elo for chess, and is now the backbone of
// competitive ranking systems everywhere (chess, League of Legends, etc.).
//
// THE CORE IDEA:
// Before a match, Elo predicts the probability of each player winning
// based on the difference in their ratings. After the match, both ratings
// are adjusted — the actual outcome is compared against the expectation.
//
// KEY FORMULA COMPONENTS:
//
//   Expected Score (Ea):
//     Ea = 1 / (1 + 10^((Rb - Ra) / 400))
//
//   If Ra == Rb → Ea = 0.5 (50% chance of winning — fair match)
//   If Ra is much higher → Ea approaches 1.0 (almost certain to win)
//   If Ra is much lower  → Ea approaches 0.0 (almost certain to lose)
//
//   New Rating:
//     Ra_new = Ra + K * (Actual_Score - Expected_Score)
//
//   Actual_Score = 1.0 if you won, 0.0 if you lost.
//   K-Factor = how much ratings can shift per match.
//
// THE K-FACTOR (K=32):
// A higher K makes ratings more volatile (faster to change, great for active
// players). A lower K makes them more stable (used in grandmaster chess).
// K=32 is the standard for online platforms with frequent matches.
//
// EXAMPLE:
//   Player A: 1000 Elo, Player B: 1000 Elo (perfectly matched)
//   A wins → Ea was 0.5, actual = 1.0
//   A gains: 32 * (1.0 - 0.5) = +16 Elo → A becomes 1016
//   B loses: 32 * (0.0 - 0.5) = -16 Elo → B becomes 984
//
//   Player A: 1200 Elo, Player B: 800 Elo (A is heavily favoured)
//   B upsets A → B's Ea was ~0.09, actual = 1.0
//   B gains: 32 * (1.0 - 0.09) = +29 Elo → B becomes 829
//   A loses: 32 * (0.0 - 0.91) = -29 Elo → A becomes 1171
//   Upsets are rewarded more because they're less expected!
// ═════════════════════════════════════════════════════════════════════════════

const KFactor = 32.0

// CalculateElo returns the new ratings for Player A and Player B.
// winner: 1 if Player A won, 2 if Player B won.
func CalculateElo(ratingA, ratingB int, winner int) (int, int) {
	rA := float64(ratingA)
	rB := float64(ratingB)

	// Expected scores using the Elo formula
	expectedA := 1.0 / (1.0 + math.Pow(10, (rB-rA)/400.0))
	expectedB := 1.0 / (1.0 + math.Pow(10, (rA-rB)/400.0))

	var scoreA, scoreB float64
	if winner == 1 {
		scoreA = 1.0 // A won
		scoreB = 0.0 // B lost
	} else {
		scoreA = 0.0
		scoreB = 1.0
	}

	newRatingA := int(math.Round(rA + KFactor*(scoreA-expectedA)))
	newRatingB := int(math.Round(rB + KFactor*(scoreB-expectedB)))

	return newRatingA, newRatingB
}

// rankTierFromElo maps an Elo rating to a display tier name.
// These thresholds mirror common competitive game rank tiers.
func rankTierFromElo(elo int) string {
	switch {
	case elo >= 2000:
		return "Grandmaster"
	case elo >= 1700:
		return "Master"
	case elo >= 1400:
		return "Diamond"
	case elo >= 1200:
		return "Platinum"
	case elo >= 1100:
		return "Gold"
	case elo >= 1000:
		return "Silver"
	default:
		return "Bronze"
	}
}

// UpdateEloRatings fetches both players' current ratings, calculates new ratings,
// and updates them in the database atomically.
//
// WHY ATOMICALLY?
// A match completion triggers two consecutive DB updates (winner and loser).
// If the app crashed between those two writes, one player's rating would update
// but the other's wouldn't — a corrupted state. Using a database transaction
// (BEGIN / COMMIT) ensures both updates succeed together or neither does.
func UpdateEloRatings(matchID, winnerID, loserID string) (int, int, int, int, error) {
	ctx := context.Background()

	// Start a database transaction.
	tx, err := DBPool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch current ratings for both players.
	var winnerElo, loserElo int
	err = tx.QueryRow(ctx, "SELECT elo_rating FROM users WHERE id=$1", winnerID).Scan(&winnerElo)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings fetch winner: %w", err)
	}
	err = tx.QueryRow(ctx, "SELECT elo_rating FROM users WHERE id=$1", loserID).Scan(&loserElo)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings fetch loser: %w", err)
	}

	// Calculate new ratings (winner = player 1).
	newWinnerElo, newLoserElo := CalculateElo(winnerElo, loserElo, 1)

	// Update winner: increment wins, update Elo and tier.
	_, err = tx.Exec(ctx,
		`UPDATE users SET elo_rating=$1, rank_tier=$2, wins=wins+1 WHERE id=$3`,
		newWinnerElo, rankTierFromElo(newWinnerElo), winnerID,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings update winner: %w", err)
	}

	// Update loser: increment losses, update Elo and tier.
	_, err = tx.Exec(ctx,
		`UPDATE users SET elo_rating=$1, rank_tier=$2, losses=losses+1 WHERE id=$3`,
		newLoserElo, rankTierFromElo(newLoserElo), loserID,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings update loser: %w", err)
	}

	// Log winner's rating change in history table.
	_, err = tx.Exec(ctx,
		`INSERT INTO elo_history (user_id, match_id, old_elo, new_elo) VALUES ($1, $2, $3, $4)`,
		winnerID, matchID, winnerElo, newWinnerElo,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings record winner history: %w", err)
	}

	// Log loser's rating change in history table.
	_, err = tx.Exec(ctx,
		`INSERT INTO elo_history (user_id, match_id, old_elo, new_elo) VALUES ($1, $2, $3, $4)`,
		loserID, matchID, loserElo, newLoserElo,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings record loser history: %w", err)
	}

	// Commit the transaction — both updates are now permanent.
	err = tx.Commit(ctx)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings commit tx: %w", err)
	}

	return winnerElo, newWinnerElo, loserElo, newLoserElo, nil
}

