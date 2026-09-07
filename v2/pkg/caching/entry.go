package caching

import (
	"encoding/binary"
	"errors"
)

// Entry envelope, labels ahead of the value so a hit reads them without
// touching the value:
//
//	[version byte][uvarint n]{[uvarint len][label]}*n[value...]
const entryFormatVersion byte = 1

var ErrEntryFormat = errors.New("cache entry is not in a known format")

func EncodeEntry(value []byte, labels []string) []byte {
	size := 1 + binary.MaxVarintLen64 + len(value)
	for _, label := range labels {
		size += binary.MaxVarintLen64 + len(label)
	}

	out := make([]byte, 0, size)
	out = append(out, entryFormatVersion)
	out = binary.AppendUvarint(out, uint64(len(labels)))
	for _, label := range labels {
		out = binary.AppendUvarint(out, uint64(len(label)))
		out = append(out, label...)
	}
	return append(out, value...)
}

// DecodeEntry returns the value as a subslice of b.
func DecodeEntry(b []byte) (value []byte, labels []string, err error) {
	if len(b) == 0 || b[0] != entryFormatVersion {
		return nil, nil, ErrEntryFormat
	}
	rest := b[1:]

	count, n := binary.Uvarint(rest)
	if n <= 0 || count > uint64(len(rest)) {
		return nil, nil, ErrEntryFormat
	}
	rest = rest[n:]

	if count > 0 {
		labels = make([]string, 0, count)
	}
	for i := uint64(0); i < count; i++ {
		size, n := binary.Uvarint(rest)
		if n <= 0 || size > uint64(len(rest)-n) {
			return nil, nil, ErrEntryFormat
		}
		rest = rest[n:]
		labels = append(labels, string(rest[:size]))
		rest = rest[size:]
	}

	return rest, labels, nil
}
