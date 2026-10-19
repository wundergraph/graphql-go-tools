package caching

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
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
//
// body with client directives (format 3): kind, then
// flags     1 byte, bit 0 public, bit 1 shared only
// shortfall uvarint, ClientShortfall in ms
// and then the body's keys and value as above. Format 1 reads as public with
// no shortfall, as it was served before format 3.
const (
	entryFormatBody       byte = 1
	entryFormatRecord     byte = 2
	entryFormatBodyClient byte = 3
)

const (
	entryFlagPublic     byte = 1
	entryFlagSharedOnly byte = 2
)

var ErrEntryFormat = errors.New("cache entry is not in a known format")

// EncodeItem picks the envelope the item calls for. Empty vary sets are
// dropped, since a set names at least one header and DecodeEntry refuses a
// record that says otherwise; with no set left the item is a body.
func EncodeItem(item Item) []byte {
	if sets := nonEmptySets(item.Vary); len(sets) > 0 {
		return encodeVaryRecord(sets)
	}
	return encodeClientEntry(item)
}

func encodeClientEntry(item Item) []byte {
	out := make([]byte, 0, 2+binary.MaxVarintLen64+listSize(item.SurrogateKeys)+len(item.Value))
	out = append(out, entryFormatBodyClient)
	var flags byte
	if item.Public {
		flags |= entryFlagPublic
	}
	if item.SharedOnly {
		flags |= entryFlagSharedOnly
	}
	out = append(out, flags)
	out = binary.AppendUvarint(out, uint64(max(item.ClientShortfall, 0)/time.Millisecond))
	out = appendList(out, item.SurrogateKeys)
	return append(out, item.Value...)
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

// DecodeItem decodes an entry. A body decodes to Value and SurrogateKeys, a
// record to Vary; the value is a subslice of b. Format 1 bodies read as public
// with no shortfall, as they were served before format 3.
func DecodeItem(b []byte) (Item, error) {
	if len(b) == 0 {
		return Item{}, ErrEntryFormat
	}
	kind, rest := b[0], b[1:]

	switch kind {
	case entryFormatBody:
		list, value, ok := readList(rest)
		if !ok {
			return Item{}, ErrEntryFormat
		}
		return Item{Value: value, SurrogateKeys: list, Public: true}, nil

	case entryFormatBodyClient:
		if len(rest) < 1 {
			return Item{}, ErrEntryFormat
		}
		flags := rest[0]
		shortfall, n := binary.Uvarint(rest[1:])
		if n <= 0 || shortfall > math.MaxInt64/uint64(time.Millisecond) {
			return Item{}, ErrEntryFormat
		}
		list, value, ok := readList(rest[1+n:])
		if !ok {
			return Item{}, ErrEntryFormat
		}
		return Item{
			Value:           value,
			SurrogateKeys:   list,
			Public:          flags&entryFlagPublic != 0,
			SharedOnly:      flags&entryFlagSharedOnly != 0,
			ClientShortfall: time.Duration(shortfall) * time.Millisecond,
		}, nil

	case entryFormatRecord:
		sets, n := binary.Uvarint(rest)
		if n <= 0 || sets == 0 || sets > uint64(len(rest)) {
			return Item{}, ErrEntryFormat
		}
		rest = rest[n:]
		vary := make([][]string, 0, sets)
		for range sets {
			var set []string
			var ok bool
			set, rest, ok = readList(rest)
			// A set names at least one header.
			if !ok || len(set) == 0 {
				return Item{}, ErrEntryFormat
			}
			vary = append(vary, set)
		}
		// A record is its sets and nothing after them.
		if len(rest) != 0 {
			return Item{}, ErrEntryFormat
		}
		return Item{Vary: vary}, nil

	default:
		return Item{}, ErrEntryFormat
	}
}

// DecodeEntry is DecodeItem without the client directives.
func DecodeEntry(b []byte) (value []byte, surrogateKeys []string, vary [][]string, err error) {
	item, err := DecodeItem(b)
	return item.Value, item.SurrogateKeys, item.Vary, err
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
