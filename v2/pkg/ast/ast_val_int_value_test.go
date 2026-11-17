package ast

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocument_IntValueInRange(t *testing.T) {
	const maxSafeInteger = 1<<53 - 1
	safeIntegerRange := IntRange{Min: -maxSafeInteger, Max: maxSafeInteger}
	int64Range := IntRange{Min: math.MinInt64, Max: math.MaxInt64}
	asymmetricRange := IntRange{Min: -100, Max: 1000}

	tests := []struct {
		name     string
		raw      string
		negative bool
		r        IntRange
		want     bool
	}{
		{name: "max int32", raw: "2147483647", r: Int32Range, want: true},
		{name: "min int32", raw: "2147483648", negative: true, r: Int32Range, want: true},
		{name: "above max int32", raw: "2147483648", r: Int32Range, want: false},
		{name: "below min int32", raw: "2147483649", negative: true, r: Int32Range, want: false},
		{name: "max safe integer", raw: "9007199254740991", r: safeIntegerRange, want: true},
		{name: "min safe integer", raw: "9007199254740991", negative: true, r: safeIntegerRange, want: true},
		{name: "above max safe integer", raw: "9007199254740992", r: safeIntegerRange, want: false},
		{name: "below min safe integer", raw: "9007199254740992", negative: true, r: safeIntegerRange, want: false},
		{name: "asymmetric rejects negative magnitude within max", raw: "500", negative: true, r: asymmetricRange, want: false},
		{name: "asymmetric accepts positive", raw: "500", r: asymmetricRange, want: true},
		{name: "max int64", raw: "9223372036854775807", r: int64Range, want: true},
		{name: "min int64", raw: "9223372036854775808", negative: true, r: int64Range, want: true},
		{name: "above max int64", raw: "9223372036854775808", r: int64Range, want: false},
		{name: "below min int64", raw: "9223372036854775809", negative: true, r: int64Range, want: false},
		{name: "beyond uint64", raw: "18446744073709551616", r: int64Range, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := NewSmallDocument()
			ref := doc.ImportIntValue([]byte(tt.raw), tt.negative)
			assert.Equal(t, tt.want, doc.IntValueInRange(ref, tt.r))
		})
	}
}

func TestDocument_IntValueValidInt32(t *testing.T) {
	doc := NewSmallDocument()
	assert.True(t, doc.IntValueValidInt32(doc.ImportIntValue([]byte("2147483648"), true)))
	assert.False(t, doc.IntValueValidInt32(doc.ImportIntValue([]byte("2147483648"), false)))
}

func TestIntRange_OrDefault(t *testing.T) {
	assert.Equal(t, Int32Range, IntRange{}.OrDefault())
	custom := IntRange{Min: -5, Max: 5}
	assert.Equal(t, custom, custom.OrDefault())
}
