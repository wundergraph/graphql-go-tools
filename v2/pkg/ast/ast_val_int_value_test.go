package ast

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocument_IntValueInRange(t *testing.T) {
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
		{name: "beyond uint64", raw: "18446744073709551616", r: Int32Range, want: false},
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
