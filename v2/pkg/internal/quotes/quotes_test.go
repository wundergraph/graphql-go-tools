package quotes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapBytes(t *testing.T) {
	testCases := []struct {
		s    []byte
		want []byte
	}{
		{nil, []byte(`""`)},
		{[]byte("foo"), []byte(`"foo"`)},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(string(tc.s), func(t *testing.T) {
			r := WrapBytes(tc.s)
			assert.Equal(t, tc.want, r)
		})
	}
}
