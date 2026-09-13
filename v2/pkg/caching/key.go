package caching

import (
	"crypto/sha256"
	"encoding/hex"
)

// keyFormatVersion leads every key this package builds. A change to the key
// layout, to the meaning of any digest, or to the stored entry format (see
// entry.go) bumps it, which orphans the entries written under the old layout
// instead of letting them be read back as something they are not. Orphaned
// entries are not deleted, they simply stop being asked for and fall out on
// their own TTL.
//
// Layout: version ':' entity ':' selection, plus ':' privateID for an entry
// that belongs to one user, each a Digest in hex. The segment count keeps the
// two forms apart, so a private key never reads as a public one.
const keyFormatVersion = "v3"

const (
	digestHex     = 2 * sha256.Size
	keyLen        = len(keyFormatVersion) + 1 + digestHex + 1 + digestHex
	privateKeyLen = keyLen + 1 + digestHex
)

// Digest is the SHA-256 of one input to a key. Collision resistant, so two
// inputs cannot end up on one key, and fixed size, so raw inputs never reach
// the store.
type Digest [sha256.Size]byte

func DigestBytes(b []byte) Digest {
	return sha256.Sum256(b)
}

func DigestString(s string) Digest {
	return sha256.Sum256([]byte(s))
}

// DigestParts digests the parts in order with a zero byte between them, so a
// byte moving from the end of one part to the start of the next cannot go
// unnoticed.
func DigestParts(parts ...[]byte) Digest {
	h := sha256.New()
	for i, part := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write(part)
	}
	var d Digest
	h.Sum(d[:0])
	return d
}

// Key builds the cache key for one entity within one fetch.
func Key(entity, selection Digest) string {
	buf := make([]byte, 0, keyLen)
	buf = appendKey(buf, entity, selection)
	return string(buf)
}

// PrivateKey builds the key for one entity within one fetch as seen by one
// user.
func PrivateKey(entity, selection, privateID Digest) string {
	buf := make([]byte, 0, privateKeyLen)
	buf = appendKey(buf, entity, selection)
	buf = append(buf, ':')
	buf = hex.AppendEncode(buf, privateID[:])
	return string(buf)
}

func appendKey(buf []byte, entity, selection Digest) []byte {
	buf = append(buf, keyFormatVersion...)
	buf = append(buf, ':')
	buf = hex.AppendEncode(buf, entity[:])
	buf = append(buf, ':')
	buf = hex.AppendEncode(buf, selection[:])
	return buf
}
