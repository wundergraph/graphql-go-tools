package caching

import (
	"encoding/binary"
	"encoding/hex"
)

// keyFormatVersion leads every key this package builds. A change to the key
// layout, to the meaning of any hash, or to the stored entry format (see
// entry.go) bumps it, which orphans the entries written under the old layout
// instead of letting them be read back as something they are not. Orphaned
// entries are not deleted, they simply stop being asked for and fall out on
// their own TTL.
//
// Layout: version ':' entityHash ':' selectionHash, plus ':' privateIDHash for
// an entry that belongs to one user. The segment count keeps the two forms
// apart, so a private key never reads as a public one.
const keyFormatVersion = "v2"

const (
	hexDigits     = 16
	keyLen        = len(keyFormatVersion) + 1 + hexDigits + 1 + hexDigits
	privateKeyLen = keyLen + 1 + hexDigits
)

// Key builds the cache key for one entity within one fetch.
func Key(entityHash, selectionHash uint64) string {
	buf := make([]byte, 0, keyLen)
	buf = appendKey(buf, entityHash, selectionHash)
	return string(buf)
}

// PrivateKey builds the key for one entity within one fetch as seen by one
// user. privateIDHash is the hash of the user id, so raw ids never reach the
// store.
func PrivateKey(entityHash, selectionHash, privateIDHash uint64) string {
	buf := make([]byte, 0, privateKeyLen)
	buf = appendKey(buf, entityHash, selectionHash)
	buf = append(buf, ':')
	buf = appendHex64(buf, privateIDHash)
	return string(buf)
}

func appendKey(buf []byte, entityHash, selectionHash uint64) []byte {
	buf = append(buf, keyFormatVersion...)
	buf = append(buf, ':')
	buf = appendHex64(buf, entityHash)
	buf = append(buf, ':')
	buf = appendHex64(buf, selectionHash)
	return buf
}

func appendHex64(dst []byte, fromInt uint64) []byte {
	fromByte := make([]byte, 8)
	binary.BigEndian.PutUint64(fromByte, fromInt)
	return hex.AppendEncode(dst, fromByte)
}
