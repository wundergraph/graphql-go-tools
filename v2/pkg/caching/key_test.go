package caching

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKey(t *testing.T) {
	t.Parallel()

	entity := DigestString("entity")
	selection := DigestString("selection")
	u1 := DigestString("u1")

	public := Key(entity, selection)
	private := PrivateKey(entity, selection, u1)

	assert.Equal(t, "v3:"+hex.EncodeToString(entity[:])+":"+hex.EncodeToString(selection[:]), public)
	assert.Len(t, public, keyLen)
	assert.True(t, strings.HasPrefix(private, public+":"))
	assert.Len(t, private, privateKeyLen)
	assert.Equal(t, hex.EncodeToString(u1[:]), strings.TrimPrefix(private, public+":"))

	assert.Equal(t, Digest(sha256.Sum256([]byte("u1"))), u1)
	assert.Equal(t, DigestBytes([]byte("u1")), u1)
	assert.Equal(t, public, Key(entity, selection))
	assert.Equal(t, private, PrivateKey(entity, selection, u1))

	assert.NotEqual(t, private, PrivateKey(entity, selection, DigestString("u2")))
	assert.NotEqual(t, public, Key(selection, entity))
}

func TestDigestParts(t *testing.T) {
	t.Parallel()

	assert.Equal(t, DigestParts([]byte("ab"), []byte("c")), DigestParts([]byte("ab"), []byte("c")))
	assert.NotEqual(t, DigestParts([]byte("ab"), []byte("c")), DigestParts([]byte("a"), []byte("bc")),
		"a byte crossing the boundary changes the digest")
	assert.Equal(t, DigestParts([]byte("abc")), DigestBytes([]byte("abc")),
		"a single part carries no separator")
}

// Two ids the previous 64-bit key hash could not tell apart. Found by
// cycle-finding over xxhash, and pinned here so the assertion below stays
// honest if the digest ever changes.
const (
	collidingIDA = "user-ba3756407998bfc3"
	collidingIDB = "user-70ca41f165edbbb0"
)

func TestKeyCollisionResistance(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, collidingIDA, collidingIDB)
	require.Equal(t, xxhash.Sum64String(collidingIDA), xxhash.Sum64String(collidingIDB),
		"the pair must collide under the old hash, or this test proves nothing")

	entity, selection := DigestString("entity"), DigestString("selection")

	// The same bytes as a user id, an entity representation and a selection.
	assert.NotEqual(t, PrivateKey(entity, selection, DigestString(collidingIDA)), PrivateKey(entity, selection, DigestString(collidingIDB)))
	assert.NotEqual(t, Key(DigestBytes([]byte(collidingIDA)), selection), Key(DigestBytes([]byte(collidingIDB)), selection))
	assert.NotEqual(t, Key(entity, DigestParts([]byte(collidingIDA), nil)), Key(entity, DigestParts([]byte(collidingIDB), nil)))
}
