package caching

import (
	"encoding/binary"
	"errors"
)

// Entry envelope, header tags ahead of the value so a hit reads them without
// touching the value:
//
//	[version byte][uvarint n]{[uvarint len][tag]}*n[value...]
const entryFormatVersion byte = 1

var ErrEntryFormat = errors.New("cache entry is not in a known format")

func EncodeEntry(value []byte, headerTags []string) []byte {
	size := 1 + binary.MaxVarintLen64 + len(value)
	for _, headerTag := range headerTags {
		size += binary.MaxVarintLen64 + len(headerTag)
	}

	out := make([]byte, 0, size)
	out = append(out, entryFormatVersion)
	out = binary.AppendUvarint(out, uint64(len(headerTags)))
	for _, headerTag := range headerTags {
		out = binary.AppendUvarint(out, uint64(len(headerTag)))
		out = append(out, headerTag...)
	}
	return append(out, value...)
}

// DecodeEntry returns the value as a subslice of b.
func DecodeEntry(b []byte) (value []byte, headerTags []string, err error) {
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
		headerTags = append(headerTags, string(rest[:size]))
		rest = rest[size:]
	}

	return rest, headerTags, nil
}
