package main

import "math"

const KFactor = 32.0 // Standard K-factor for rapid rating adjustments

// CalculateElo returns the new ratings for Player A and Player B.
// winner: 1 if Player A won, 2 if Player B won.
func CalculateElo(ratingA, ratingB int, winner int) (int, int) {
	rA := float64(ratingA)
	rB := float64(ratingB)

	// Expected scores
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

	// Calculate updated ratings
	newRatingA := int(math.Round(rA + KFactor*(scoreA-expectedA)))
	newRatingB := int(math.Round(rB + KFactor*(scoreB-expectedB)))

	return newRatingA, newRatingB
}
