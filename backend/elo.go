package main

import (
	"context"
	"fmt"
	"math"
)

const KFactor = 32.0

func CalculateElo(ratingA, ratingB int, winner int) (int, int) {
	rA := float64(ratingA)
	rB := float64(ratingB)

	expectedA := 1.0 / (1.0 + math.Pow(10, (rB-rA)/400.0))
	expectedB := 1.0 / (1.0 + math.Pow(10, (rA-rB)/400.0))

	var scoreA, scoreB float64
	if winner == 1 {
		scoreA = 1.0
		scoreB = 0.0
	} else {
		scoreA = 0.0
		scoreB = 1.0
	}

	newRatingA := int(math.Round(rA + KFactor*(scoreA-expectedA)))
	newRatingB := int(math.Round(rB + KFactor*(scoreB-expectedB)))

	return newRatingA, newRatingB
}

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

func UpdateEloRatings(matchID, winnerID, loserID string) (int, int, int, int, error) {
	ctx := context.Background()

	tx, err := DBPool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var winnerElo, loserElo int
	err = tx.QueryRow(ctx, "SELECT elo_rating FROM users WHERE id=$1", winnerID).Scan(&winnerElo)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings fetch winner: %w", err)
	}
	err = tx.QueryRow(ctx, "SELECT elo_rating FROM users WHERE id=$1", loserID).Scan(&loserElo)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings fetch loser: %w", err)
	}

	newWinnerElo, newLoserElo := CalculateElo(winnerElo, loserElo, 1)

	_, err = tx.Exec(ctx,
		`UPDATE users SET elo_rating=$1, rank_tier=$2, wins=wins+1 WHERE id=$3`,
		newWinnerElo, rankTierFromElo(newWinnerElo), winnerID,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings update winner: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE users SET elo_rating=$1, rank_tier=$2, losses=losses+1 WHERE id=$3`,
		newLoserElo, rankTierFromElo(newLoserElo), loserID,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings update loser: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO elo_history (user_id, match_id, old_elo, new_elo) VALUES ($1, $2, $3, $4)`,
		winnerID, matchID, winnerElo, newWinnerElo,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings record winner history: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO elo_history (user_id, match_id, old_elo, new_elo) VALUES ($1, $2, $3, $4)`,
		loserID, matchID, loserElo, newLoserElo,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings record loser history: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("UpdateEloRatings commit tx: %w", err)
	}

	return winnerElo, newWinnerElo, loserElo, newLoserElo, nil
}
