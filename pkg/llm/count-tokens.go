package llm

import (
	"unicode/utf8"
)

// EstimateTokens estimates the number of tokens in a string.
// This is a very simplified approach and might not be accurate for all models.
// More accurate tokenization requires model-specific libraries.
func EstimateTokens(text string) int { return utf8.RuneCountInString(text) / 4 }
