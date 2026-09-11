package caching

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKey(t *testing.T) {
	t.Parallel()

	public := Key(1, 2)
	private := PrivateKey(1, 2, 3)

	assert.Equal(t, "v2:0000000000000001:0000000000000002", public)
	assert.Equal(t, "v2:0000000000000001:0000000000000002:0000000000000003", private)
	assert.True(t, strings.HasPrefix(private, public+":"))

	assert.Equal(t, public, Key(1, 2))
	assert.Equal(t, private, PrivateKey(1, 2, 3))

	assert.NotEqual(t, private, PrivateKey(1, 2, 4))
	assert.NotEqual(t, private, PrivateKey(2, 1, 3))
	assert.NotEqual(t, public, Key(2, 1))
}
