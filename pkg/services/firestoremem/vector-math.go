package firestoremem

import (
	"log/slog"
	"math"

	"cloud.google.com/go/firestore"
)

func defaultNodeThresholds(nc int) []float64 {
	thresholds := make([]float64, nc)
	for i := range thresholds {
		if i == 0 {
			thresholds[i] = 0.3
		} else {
			thresholds[i] = 0.1
		}
	}
	return thresholds
}

// normalizeVector normalizes a vector to have unit length
func normalizeVector(vector firestore.Vector32) firestore.Vector32 {
	if len(vector) == 0 {
		return vector
	}

	var magnitude float64
	for _, val := range vector {
		magnitude += float64(val * val)
	}
	magnitude = math.Sqrt(magnitude)

	if magnitude == 0 {
		return vector
	}

	normalized := make(firestore.Vector32, len(vector))
	for i, val := range vector {
		normalized[i] = float32(float64(val) / magnitude)
	}

	return normalized
}

// combineVectors adds multiple vectors together and normalizes the result
func combineVectors(vectors []firestore.Vector32) firestore.Vector32 {
	if len(vectors) == 0 {
		return nil
	}
	vectorLen := len(vectors[0])
	if vectorLen == 0 {
		return nil
	}
	combined := make(firestore.Vector32, vectorLen)
	for _, vec := range vectors {
		if len(vec) != vectorLen {
			slog.Warn("vector length mismatch in combineVectors", "expected", vectorLen, "got", len(vec))
			continue
		}
		for i, val := range vec {
			combined[i] += val
		}
	}
	return normalizeVector(combined)
}
