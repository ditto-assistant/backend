package numfmt

import (
	"fmt"
	"testing"
)

func TestUSD(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{1, "$1.00"},
		{123456789, "$123456789.00"},
		{123456789.123, "$123456789.12"},
		{123456789.123456, "$123456789.12"},
		{123456789.123456789, "$123456789.12"},
		{-1, "-$1.00"},
		{-123456.789, "-$123456.79"},
		{-0.01, "-$0.01"},
		{-0.001, "-$0.00"},
		{0, "$0.00"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("USD(%f)", test.input), func(t *testing.T) {
			if test.expected != USD(test.input) {
				t.Errorf("USD(%f) = %s; expected %s", test.input, USD(test.input), test.expected)
			}
		})
	}
}

func TestFormatLargeNumber(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{123456789, "123.46M"},
		{-123456789, "-123.46M"},
		{1234567890, "1.23B"},
		{-1234567890, "-1.23B"},
		{1000, "1.00K"},
		{0, "0"},
		{-1000, "-1.00K"},
		{1000000, "1.00M"},
		{-1000000, "-1.00M"},
		{1000000000, "1.00B"},
		{-1000000000, "-1.00B"},
		{1000000000000, "1.00T"},
		{-1000000000000, "-1.00T"},
		{1000000000000000, "1.00Q"},
		{-1000000000000000, "-1.00Q"},
		{999, "999"},
		{-999, "-999"},
		{1001, "1.00K"},
		{-1001, "-1.00K"},
		{1500000, "1.50M"},
		{-1500000, "-1.50M"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("FormatLargeNumber(%d)", test.input), func(t *testing.T) {
			if test.expected != LargeNumber(test.input) {
				t.Errorf("FormatLargeNumber(%d) = %s; expected %s", test.input, LargeNumber(test.input), test.expected)
			}
		})
	}
}
