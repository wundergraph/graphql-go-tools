package resolve

import (
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// TransactionBeginner opens CacheTransactions over the loader's shared JSON
// arena. The loader hands one to every cache hook input; a hook opens exactly
// ONE transaction via Begin at the top of its arena work and releases it with
// Commit (via defer). A TransactionBeginner must never be retained past the
// hook it was handed to, and must never be used from the off-lock load phase.
type TransactionBeginner interface {
	// Begin enters the locked merge region and returns the transaction through
	// which all arena/parser ops run. It acquires DataBuffer.Lock once for the
	// caller; the matching Commit releases it.
	Begin() *CacheTransaction
}

// CacheTransaction is the scoped, lock-held handle for one multi-op arena
// sequence (candidate parse -> merge synthesis -> reorder, then the splice or
// write). Every arena/parser op is a method ON the transaction, so the arena
// cannot be touched without a held transaction. One transaction per hook is
// one DataBuffer.Lock acquisition; Commit MUST be called (via defer) exactly
// once.
type CacheTransaction struct {
	a  arena.Arena
	db *DataBuffer
}

// ParseBytes parses b onto the transaction's arena (lazy candidate parse).
func (t *CacheTransaction) ParseBytes(b []byte) (*astjson.Value, error) {
	return astjson.ParseBytesWithArena(t.a, b)
}

// StructuralCopy deep-copies v onto the arena, avoiding merge aliasing
// corruption when the same cached value is spliced into multiple targets.
// Callers rely on VALUE isolation (mutating the source or the copy never
// affects the other) — in heap mode (nil arena, one production resolve entry
// runs this way) astjson.DeepCopy is an identity passthrough, so a real copy
// is forced via a marshal round-trip there.
func (t *CacheTransaction) StructuralCopy(v *astjson.Value) *astjson.Value {
	if t.a != nil {
		return astjson.DeepCopy(t.a, v)
	}
	if v == nil {
		return nil
	}
	copied, err := astjson.ParseBytes(v.MarshalTo(nil))
	if err != nil {
		return v
	}
	return copied
}

// MergeValues merges src into dst on the arena. The underlying "changed" flag
// is intentionally discarded, matching how the defer merge path discards it.
func (t *CacheTransaction) MergeValues(dst, src *astjson.Value) (*astjson.Value, error) {
	merged, _, err := astjson.MergeValues(t.a, dst, src)
	return merged, err
}

// MergeValuesWithPath merges src into dst at path on the arena.
func (t *CacheTransaction) MergeValuesWithPath(dst, src *astjson.Value, path ...string) (*astjson.Value, error) {
	merged, _, err := astjson.MergeValuesWithPath(t.a, dst, src, path...)
	return merged, err
}

// NewObject allocates an empty object value on the arena.
func (t *CacheTransaction) NewObject() *astjson.Value {
	return astjson.ObjectValue(t.a)
}

// NewArray allocates an empty array value on the arena.
func (t *CacheTransaction) NewArray() *astjson.Value {
	return astjson.ArrayValue(t.a)
}

// String allocates a string value on the arena.
func (t *CacheTransaction) String(s string) *astjson.Value {
	return astjson.StringValue(t.a, s)
}

// Null returns the JSON null value.
func (t *CacheTransaction) Null() *astjson.Value {
	return astjson.NullValue
}

// Commit releases DataBuffer.Lock and ends the transaction; call via defer.
func (t *CacheTransaction) Commit() {
	t.db.Unlock()
}

// NewTransactionBeginner builds a TransactionBeginner over an arena and its
// DataBuffer guard. The loader wires its own internally; this constructor
// exists for cache-controller unit tests and out-of-loader tooling that must
// exercise the transaction contract directly.
func NewTransactionBeginner(a arena.Arena, db *DataBuffer) TransactionBeginner {
	return cacheTransactionBeginner{a: a, db: db}
}

// cacheTransactionBeginner is the loader-backed TransactionBeginner, bound to
// the request's shared jsonArena and its DataBuffer guard. It is built only
// when a cache hook actually runs, never on the no-op path.
type cacheTransactionBeginner struct {
	a  arena.Arena
	db *DataBuffer
}

func (b cacheTransactionBeginner) Begin() *CacheTransaction {
	b.db.Lock()
	return &CacheTransaction{a: b.a, db: b.db}
}
