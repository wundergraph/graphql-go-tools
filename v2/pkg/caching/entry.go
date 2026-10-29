package caching

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// Entry envelope. The first byte says what follows: a body carries its
// surrogate keys ahead of the value so a hit reads them without touching the
// value, a vary record carries the header name sets its variants are stored
// under, newest first.
// kind      1 byte, entryFormatBody or entryFormatRecord
// body:
// flags     1 byte, bit 0 public, bit 1 shared only
// gap       uvarint, TTL less ClientTTL in ms; both count down together
// count     uvarint, number of surrogate keys
// keys      count times: 1 byte tier, uvarint byte length, then the key, that many bytes
// value     everything after the last key, the cached body itself, no length prefix
// record:
// sets      uvarint, number of name sets, at least one
// names     sets times: uvarint count, then count names, each uvarint length prefixed
//
// Example: public Value {"id":42} with keys "subgraph-users" and declared "user-42"
// 04                   kind: body
// 01                   flags: public
// 00                   gap: none
// 02                   count: 2 keys
// 00 0e subgraph-users tier subgraph, length 14, then the bytes
// 02 07 user-42        tier declared, length 7, then the bytes
// {"id":42}            value, afterwards
//
// Kinds 1 and 3, bodies without flags, are retired and refused.
const (
	entryFormatRecord byte = 2
	entryFormatBody   byte = 4
)

const (
	entryFlagPublic     byte = 1
	entryFlagSharedOnly byte = 2
)

var ErrEntryFormat = errors.New("cache entry is not in a known format")

// EncodeItem picks the envelope the item calls for. Empty vary sets are
// dropped, since a set names at least one header and DecodeEntry refuses a
// record that says otherwise; with no set left the item is a body. A key with
// an unknown tier is refused, as DecodeEntry would refuse the entry.
func EncodeItem(item Item) ([]byte, error) {
	if sets := nonEmptySets(item.Vary); len(sets) > 0 {
		return encodeVaryRecord(sets), nil
	}
	for _, surrogateKey := range item.SurrogateKeys {
		if !surrogateKey.Tier.valid() {
			return nil, fmt.Errorf("%w: %d, key %q", ErrInvalidSurrogateKeyTier, surrogateKey.Tier, item.Key)
		}
	}
	return encodeBody(item), nil
}

func encodeBody(item Item) []byte {
	size := 2 + 2*binary.MaxVarintLen64 + len(item.Value)
	for _, surrogateKey := range item.SurrogateKeys {
		size += 1 + binary.MaxVarintLen64 + len(surrogateKey.Value)
	}
	out := make([]byte, 0, size)
	out = append(out, entryFormatBody)
	var flags byte
	if item.Public {
		flags |= entryFlagPublic
	}
	if item.SharedOnly {
		flags |= entryFlagSharedOnly
	}
	out = append(out, flags)
	out = binary.AppendUvarint(out, uint64(max(item.TTL-item.ClientTTL, 0)/time.Millisecond))
	out = binary.AppendUvarint(out, uint64(len(item.SurrogateKeys)))
	for _, surrogateKey := range item.SurrogateKeys {
		out = append(out, byte(surrogateKey.Tier))
		out = binary.AppendUvarint(out, uint64(len(surrogateKey.Value)))
		out = append(out, surrogateKey.Value...)
	}
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

// DecodeItem decodes an entry with ttl left to live. A body decodes to Value
// and SurrogateKeys, a record to Vary; the value is a subslice of b.
func DecodeItem(b []byte, ttl time.Duration) (Item, error) {
	if len(b) == 0 {
		return Item{}, ErrEntryFormat
	}
	kind, rest := b[0], b[1:]

	switch kind {
	case entryFormatBody:
		if len(rest) < 1 {
			return Item{}, ErrEntryFormat
		}
		flags := rest[0]
		gap, n := binary.Uvarint(rest[1:])
		if n <= 0 || gap > math.MaxInt64/uint64(time.Millisecond) {
			return Item{}, ErrEntryFormat
		}
		surrogateKeys, value, ok := readTieredList(rest[1+n:])
		if !ok {
			return Item{}, ErrEntryFormat
		}
		return Item{
			Value:         value,
			TTL:           ttl,
			SurrogateKeys: surrogateKeys,
			Public:        flags&entryFlagPublic != 0,
			SharedOnly:    flags&entryFlagSharedOnly != 0,
			ClientTTL:     max(ttl-time.Duration(gap)*time.Millisecond, 0),
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
		return Item{Vary: vary, TTL: ttl}, nil

	default:
		return Item{}, ErrEntryFormat
	}
}

// DecodeEntry is DecodeItem without the client directives.
func DecodeEntry(b []byte) (value []byte, surrogateKeys []SurrogateKey, vary [][]string, err error) {
	item, err := DecodeItem(b, 0)
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

// readTieredList reads a body's keys off b and returns what follows them.
func readTieredList(b []byte) (list []SurrogateKey, rest []byte, ok bool) {
	count, n := binary.Uvarint(b)
	if n <= 0 || count > uint64(len(b)) {
		return nil, nil, false
	}
	rest = b[n:]

	for range count {
		if len(rest) == 0 {
			return nil, nil, false
		}
		tier := SurrogateKeyTier(rest[0])
		if !tier.valid() {
			return nil, nil, false
		}
		rest = rest[1:]
		size, n := binary.Uvarint(rest)
		if n <= 0 || size > uint64(len(rest)-n) {
			return nil, nil, false
		}
		rest = rest[n:]
		list = append(list, SurrogateKey{Value: string(rest[:size]), Tier: tier})
		rest = rest[size:]
	}
	return list, rest, true
}
