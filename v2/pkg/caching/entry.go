package caching

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Entry envelope. The first byte says what follows: a body carries its
// surrogate keys ahead of the value so a hit reads them without touching the
// value, a vary record carries the header name sets its variants are stored
// under, newest first.
// kind      1 byte, entryFormatTieredBody or entryFormatRecord
// tiered body:
// count     uvarint, number of surrogate keys
// keys      count times: 1 byte tier, uvarint byte length, then the key, that many bytes
// value     everything after the last key, the cached body itself, no length prefix
// record:
// sets      uvarint, number of name sets, at least one
// names     sets times: uvarint count, then count names, each uvarint length prefixed
//
// Example: Value {"id":42} with keys "subgraph-users" and declared "user-42"
// 03                   kind: tiered body
// 02                   count: 2 keys
// 00 0e subgraph-users tier subgraph, length 14, then the bytes
// 02 07 user-42        tier declared, length 7, then the bytes
// {"id":42}            value, afterwards
//
// entryFormatBody is the untiered body that came before: keys are only length
// prefixed and read back with a tier guessed from their prefix. Never written.
const (
	entryFormatBody       byte = 1
	entryFormatRecord     byte = 2
	entryFormatTieredBody byte = 3
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
	return encodeEntry(item.Value, item.SurrogateKeys), nil
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

func encodeEntry(value []byte, surrogateKeys []SurrogateKey) []byte {
	size := 1 + binary.MaxVarintLen64 + len(value)
	for _, surrogateKey := range surrogateKeys {
		size += 1 + binary.MaxVarintLen64 + len(surrogateKey.Value)
	}
	out := make([]byte, 0, size)
	out = append(out, entryFormatTieredBody)
	out = binary.AppendUvarint(out, uint64(len(surrogateKeys)))
	for _, surrogateKey := range surrogateKeys {
		out = append(out, byte(surrogateKey.Tier))
		out = binary.AppendUvarint(out, uint64(len(surrogateKey.Value)))
		out = append(out, surrogateKey.Value...)
	}
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
func DecodeEntry(b []byte) (value []byte, surrogateKeys []SurrogateKey, vary [][]string, err error) {
	if len(b) == 0 {
		return nil, nil, nil, ErrEntryFormat
	}
	kind, rest := b[0], b[1:]

	switch kind {
	case entryFormatTieredBody:
		surrogateKeys, rest, ok := readTieredList(rest)
		if !ok {
			return nil, nil, nil, ErrEntryFormat
		}
		return rest, surrogateKeys, nil, nil

	case entryFormatBody:
		list, rest, ok := readList(rest)
		if !ok {
			return nil, nil, nil, ErrEntryFormat
		}
		for _, s := range list {
			surrogateKeys = append(surrogateKeys, SurrogateKey{Value: s, Tier: legacySurrogateKeyTier(s)})
		}
		return rest, surrogateKeys, nil, nil

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

// readTieredList reads a tiered body's keys off b and returns what follows them.
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
