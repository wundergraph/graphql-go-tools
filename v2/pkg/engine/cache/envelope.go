package cache

import (
	"strconv"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The value envelope: every L2 entry is stored as
//
//	{"data":<normalized value>,
//	 "cc":{"ttl":60,"created":1785852117,"scope":"public"}}
//
// `data` is the value the read path serves and `cc` carries the freshness
// metadata (the TTL the entry was written with, the unix second it was written
// at, and its privacy scope). L1 never sees an envelope — it holds decoded
// values.

const (
	// cacheScopePublic marks an entry any requester may be served.
	cacheScopePublic = "public"
	// cacheScopePrivate marks an entry that belongs to the ONE requester whose
	// identity partitions its key. Reading it through a key derivation of the
	// other scope is configuration drift and discards the entry.
	cacheScopePrivate = "private"
)

// cacheControl is one entry's freshness and privacy metadata.
type cacheControl struct {
	// TTL is the lifetime the entry was written with; it is stored in whole
	// seconds.
	TTL time.Duration
	// Created is the wall-clock write moment, stored as unix seconds, so
	// remaining freshness stays computable when the store cannot report it.
	Created time.Time
	// Scope is the entry's privacy scope.
	Scope string
}

// storedEnvelope is one decoded L2 value: the data to serve, the scope it was
// written under, and the entry's own freshness record.
type storedEnvelope struct {
	Data  *astjson.Value
	Scope string
	// TTL is the lifetime the entry was written with and Created the moment of
	// the write, so what is LEFT of the entry's freshness is computable from the
	// entry alone — the store's own remaining-TTL report is optional (0 means
	// unknown) and would read as "expired" wherever a backend does not keep one.
	TTL     time.Duration
	Created time.Time
}

// encodeEnvelope renders one L2 value. A nil data writes the negative sentinel
// (`"data":null`). This is the ONE place values become stored bytes.
func encodeEnvelope(data *astjson.Value, cc cacheControl) []byte {
	buf := make([]byte, 0, 128)
	buf = append(buf, `{"data":`...)
	if data == nil {
		buf = append(buf, "null"...)
	} else {
		buf = data.MarshalTo(buf)
	}
	buf = append(buf, `,"cc":{"ttl":`...)
	buf = strconv.AppendInt(buf, int64(cc.TTL/time.Second), 10)
	buf = append(buf, `,"created":`...)
	buf = strconv.AppendInt(buf, cc.Created.Unix(), 10)
	buf = append(buf, `,"scope":"`...)
	buf = append(buf, cc.Scope...)
	buf = append(buf, `"}}`...)
	return buf
}

// decodeEnvelope parses one stored value. ok=false when the bytes are not a
// decodable envelope — unparseable or foreign content is a MISS, never an
// error: the fetch falls back to the origin and its write replaces the entry.
// Entries written under an older format are unreachable by construction (the
// format version leads every key), so a failure here means corruption.
func decodeEnvelope(tx *resolve.CacheTransaction, raw []byte) (storedEnvelope, bool) {
	value, err := tx.ParseBytes(raw)
	if err != nil || value.Type() != astjson.TypeObject {
		return storedEnvelope{}, false
	}
	data := value.Get("data")
	if data == nil {
		return storedEnvelope{}, false
	}
	// Public is the scope reading that can only ever COST a hit (a private entry
	// lives under a partitioned key a public read never derives), so it is what
	// an entry that records none — a foreign or corrupt one — falls back to.
	envelope := storedEnvelope{
		Data:  data,
		Scope: cacheScopePublic,
	}
	if cc := value.Get("cc"); cc != nil {
		if scope := string(cc.GetStringBytes("scope")); scope != "" {
			envelope.Scope = scope
		}
		envelope.TTL = time.Duration(cc.GetInt64("ttl")) * time.Second
		envelope.Created = time.Unix(cc.GetInt64("created"), 0)
	}
	return envelope, true
}
