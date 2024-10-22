package numfmt

import (
	"fmt"
	"testing"
)

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
			if test.expected != FormatLargeNumber(test.input) {
				t.Errorf("FormatLargeNumber(%d) = %s; expected %s", test.input, FormatLargeNumber(test.input), test.expected)
			}
		})
	}
}
