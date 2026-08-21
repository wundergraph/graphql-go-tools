package caching

import (
	"encoding/binary"
	"encoding/hex"
)

// keyFormatVersion leads every key this package builds. A change to the layout
// or to the meaning of either hash bumps it, which orphans the entries written
// under the old layout instead of letting them be read back as something they
// are not. Orphaned entries are not deleted, they simply stop being asked for
// and fall out on their own TTL.
const keyFormatVersion = "v1"

// Key builds the cache key for one entity within one fetch.
func Key(entityHash, selectionHash uint64) string {
	const hexDigits = 16
	buf := make([]byte, 0, len(keyFormatVersion)+1+hexDigits+1+hexDigits)

	buf = append(buf, keyFormatVersion...)
	buf = append(buf, ':')
	buf = appendHex64(buf, entityHash)
	buf = append(buf, ':')
	buf = appendHex64(buf, selectionHash)

	return string(buf)
}

func appendHex64(dst []byte, fromInt uint64) []byte {
	fromByte := make([]byte, 8)
	binary.BigEndian.PutUint64(fromByte, fromInt)
	return hex.AppendEncode(dst, fromByte)
}
