package caching

import (
	"encoding/binary"
	"errors"
)

// Entry envelope. Surrogate keys sit ahead of the value so a hit reads them
// without touching the value.
// version   1 byte, currently 1
// count     uvarint, number of surrogate keys
// tags      count times: uvarint byte length, then the tag, that many bytes
// value     everything after the last tag, the cached body itself, no length prefix
//
// Example: Value {"id":42} with tags "users" and "user-42"
// 01               version
// 02               count: 2 tags
// 05 users         length 5, then the bytes
// 07 user-42       length 7, then the bytes
// {"id":42}        value, afterwards
const entryFormatVersion byte = 1

var ErrEntryFormat = errors.New("cache entry is not in a known format")

func EncodeEntry(value []byte, surrogateKeys []string) []byte {
	size := 1 + binary.MaxVarintLen64 + len(value)
	for _, surrogateKey := range surrogateKeys {
		size += binary.MaxVarintLen64 + len(surrogateKey)
	}

	out := make([]byte, 0, size)
	out = append(out, entryFormatVersion)
	out = binary.AppendUvarint(out, uint64(len(surrogateKeys)))
	for _, surrogateKey := range surrogateKeys {
		out = binary.AppendUvarint(out, uint64(len(surrogateKey)))
		out = append(out, surrogateKey...)
	}
	return append(out, value...)
}

// DecodeEntry returns the value as a subslice of b.
func DecodeEntry(b []byte) (value []byte, surrogateKeys []string, err error) {
	if len(b) == 0 || b[0] != entryFormatVersion {
		return nil, nil, ErrEntryFormat
	}
	rest := b[1:]

	count, n := binary.Uvarint(rest)
	if n <= 0 || count > uint64(len(rest)) {
		return nil, nil, ErrEntryFormat
	}
	rest = rest[n:]

	// Not sized from count: it is the entry's own claim, not yet checked
	// against its length prefixes.
	for range count {
		size, n := binary.Uvarint(rest)
		if n <= 0 || size > uint64(len(rest)-n) {
			return nil, nil, ErrEntryFormat
		}
		rest = rest[n:]
		surrogateKeys = append(surrogateKeys, string(rest[:size]))
		rest = rest[size:]
	}

	return rest, surrogateKeys, nil
}
