package caching

import (
	"encoding/binary"
	"errors"
)

// Entry envelope. The first byte says what follows: a body carries its
// surrogate keys ahead of the value so a hit reads them without touching the
// value, a vary record carries the header name sets its variants are stored
// under, newest first.
// kind      1 byte, entryFormatBody or entryFormatRecord
// body:
// count     uvarint, number of surrogate keys
// keys      count times: uvarint byte length, then the key, that many bytes
// value     everything after the last key, the cached body itself, no length prefix
// record:
// sets      uvarint, number of name sets, at least one
// names     sets times: uvarint count, then count names, each length prefixed like a key
//
// Example: Value {"id":42} with keys "users" and "user-42"
// 01               kind: body
// 02               count: 2 keys
// 05 users         length 5, then the bytes
// 07 user-42       length 7, then the bytes
// {"id":42}        value, afterwards
const (
	entryFormatBody   byte = 1
	entryFormatRecord byte = 2
)

var ErrEntryFormat = errors.New("cache entry is not in a known format")

// EncodeItem picks the envelope the item calls for. Empty vary sets are
// dropped, since a set names at least one header and DecodeEntry refuses a
// record that says otherwise; with no set left the item is a body.
func EncodeItem(item Item) []byte {
	if sets := nonEmptySets(item.Vary); len(sets) > 0 {
		return encodeVaryRecord(sets)
	}
	return encodeEntry(item.Value, item.SurrogateKeys)
}

// nonEmptySets is sets without its empty members, and sets itself when there
// are none to drop.
func nonEmptySets(sets [][]string) [][]string {
	for i, set := range sets {
		if len(set) != 0 {
			continue
		}
		kept := make([][]string, 0, len(sets)-1)
		kept = append(kept, sets[:i]...)
		for _, set := range sets[i+1:] {
			if len(set) != 0 {
				kept = append(kept, set)
			}
		}
		return kept
	}
	return sets
}

func encodeEntry(value []byte, surrogateKeys []string) []byte {
	out := make([]byte, 0, 1+listSize(surrogateKeys)+len(value))
	out = append(out, entryFormatBody)
	out = appendList(out, surrogateKeys)
	return append(out, value...)
}

func encodeVaryRecord(sets [][]string) []byte {
	size := 1 + binary.MaxVarintLen64
	for _, set := range sets {
		size += listSize(set)
	}
	out := make([]byte, 0, size)
	out = append(out, entryFormatRecord)
	out = binary.AppendUvarint(out, uint64(len(sets)))
	for _, set := range sets {
		out = appendList(out, set)
	}
	return out
}

func listSize(list []string) int {
	size := binary.MaxVarintLen64
	for _, s := range list {
		size += binary.MaxVarintLen64 + len(s)
	}
	return size
}

func appendList(out []byte, list []string) []byte {
	out = binary.AppendUvarint(out, uint64(len(list)))
	for _, s := range list {
		out = binary.AppendUvarint(out, uint64(len(s)))
		out = append(out, s...)
	}
	return out
}

// DecodeEntry returns the value as a subslice of b. Exactly one of value and
// vary is set, by the kind byte: a body decodes to value and surrogateKeys, a
// record to vary.
func DecodeEntry(b []byte) (value []byte, surrogateKeys []string, vary [][]string, err error) {
	if len(b) == 0 {
		return nil, nil, nil, ErrEntryFormat
	}
	kind, rest := b[0], b[1:]

	switch kind {
	case entryFormatBody:
		list, rest, ok := readList(rest)
		if !ok {
			return nil, nil, nil, ErrEntryFormat
		}
		return rest, list, nil, nil

	case entryFormatRecord:
		sets, n := binary.Uvarint(rest)
		if n <= 0 || sets == 0 || sets > uint64(len(rest)) {
			return nil, nil, nil, ErrEntryFormat
		}
		rest = rest[n:]
		vary = make([][]string, 0, sets)
		for range sets {
			var set []string
			var ok bool
			set, rest, ok = readList(rest)
			// A set names at least one header.
			if !ok || len(set) == 0 {
				return nil, nil, nil, ErrEntryFormat
			}
			vary = append(vary, set)
		}
		// A record is its sets and nothing after them.
		if len(rest) != 0 {
			return nil, nil, nil, ErrEntryFormat
		}
		return nil, nil, vary, nil

	default:
		return nil, nil, nil, ErrEntryFormat
	}
}

// readList reads one length prefixed list off b and returns what follows it.
func readList(b []byte) (list []string, rest []byte, ok bool) {
	count, n := binary.Uvarint(b)
	if n <= 0 || count > uint64(len(b)) {
		return nil, nil, false
	}
	rest = b[n:]

	// Not sized from count: it is the entry's own claim, not yet checked
	// against its length prefixes.
	for range count {
		size, n := binary.Uvarint(rest)
		if n <= 0 || size > uint64(len(rest)-n) {
			return nil, nil, false
		}
		rest = rest[n:]
		list = append(list, string(rest[:size]))
		rest = rest[size:]
	}
	return list, rest, true
}
