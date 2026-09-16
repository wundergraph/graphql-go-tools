package caching

import (
	"encoding/binary"
	"errors"
)

// Entry envelope. The first byte says what follows: a body carries its header
// tags ahead of the value so a hit reads them without touching the value, a
// vary record carries only the header names it varies on.
// kind      1 byte, entryFormatBody or entryFormatRecord
// count     uvarint, number of header tags, or of names for a record
// tags      count times: uvarint byte length, then the tag, that many bytes
// value     body only: everything after the last tag, the cached body itself, no length prefix
//
// Example: Value {"id":42} with tags "users" and "user-42"
// 01               kind: body
// 02               count: 2 tags
// 05 users         length 5, then the bytes
// 07 user-42       length 7, then the bytes
// {"id":42}        value, afterwards
const (
	entryFormatBody   byte = 1
	entryFormatRecord byte = 2
)

var ErrEntryFormat = errors.New("cache entry is not in a known format")

// EncodeItem picks the envelope the item calls for.
func EncodeItem(item Item) []byte {
	if len(item.Vary) > 0 {
		return EncodeVaryRecord(item.Vary)
	}
	return EncodeEntry(item.Value, item.HeaderTags)
}

func EncodeEntry(value []byte, headerTags []string) []byte {
	out := appendList(entryFormatBody, headerTags, len(value))
	return append(out, value...)
}

func EncodeVaryRecord(names []string) []byte {
	return appendList(entryFormatRecord, names, 0)
}

func appendList(kind byte, list []string, extra int) []byte {
	size := 1 + binary.MaxVarintLen64 + extra
	for _, s := range list {
		size += binary.MaxVarintLen64 + len(s)
	}

	out := make([]byte, 0, size)
	out = append(out, kind)
	out = binary.AppendUvarint(out, uint64(len(list)))
	for _, s := range list {
		out = binary.AppendUvarint(out, uint64(len(s)))
		out = append(out, s...)
	}
	return out
}

// DecodeEntry returns the value as a subslice of b. Exactly one of value and
// vary is set, by the kind byte: a body decodes to value and headerTags, a
// record to vary.
func DecodeEntry(b []byte) (value []byte, headerTags, vary []string, err error) {
	if len(b) == 0 {
		return nil, nil, nil, ErrEntryFormat
	}
	kind := b[0]
	if kind != entryFormatBody && kind != entryFormatRecord {
		return nil, nil, nil, ErrEntryFormat
	}
	rest := b[1:]

	count, n := binary.Uvarint(rest)
	if n <= 0 || count > uint64(len(rest)) {
		return nil, nil, nil, ErrEntryFormat
	}
	rest = rest[n:]

	// Not sized from count: it is the entry's own claim, not yet checked
	// against its length prefixes.
	var list []string
	for range count {
		size, n := binary.Uvarint(rest)
		if n <= 0 || size > uint64(len(rest)-n) {
			return nil, nil, nil, ErrEntryFormat
		}
		rest = rest[n:]
		list = append(list, string(rest[:size]))
		rest = rest[size:]
	}

	if kind == entryFormatRecord {
		// A record is its names and nothing after them.
		if len(rest) != 0 || len(list) == 0 {
			return nil, nil, nil, ErrEntryFormat
		}
		return nil, nil, list, nil
	}

	return rest, list, nil, nil
}
