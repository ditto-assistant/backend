package numfmt

import (
	"fmt"
	"math"
	"strconv"
)

// USD formats a float64 into a string representation using $ and two decimal places.
func USD(n float64) string {
	if n < 0 {
		return "-$" + strconv.FormatFloat(-n, 'f', 2, 64)
	}
	return "$" + strconv.FormatFloat(n, 'f', 2, 64)
}

// LargeNumber formats an int64 into a string representation using K, M, B, T, Q suffixes.
// It handles negative numbers and rounds to two decimal places.
func LargeNumber(n int64) string {
	// Handle negative numbers
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}

	if n < 1000 {
		return fmt.Sprintf("%s%d", sign, n)
	}

	suffixes := []string{"", "K", "M", "B", "T", "Q"}
	value := float64(n)
	index := 0

	for value >= 1000 && index < len(suffixes)-1 {
		value /= 1000
		index++
	}

	// Round to two decimal places
	rounded := math.Round(value*100) / 100

	return fmt.Sprintf("%s%.2f%s", sign, rounded, suffixes[index])
}
