package entitycaching

import (
	"context"
	"time"
)

type Item struct {
	Key       string
	Value     []byte
	CacheTags []string
	TTL       time.Duration
}

// Result is a single lookup outcome returned by Cache.GetMany.
type Result struct {
	// Value is only meaningful when Found is true.
	Value []byte
	// Found reports whether the key was cached, so that a cached empty value
	// stays distinguishable from a miss. The zero Result is a miss.
	Found bool
}

type Cache interface {
	// GetMany looks up every key and returns one Result per key, in the same
	// order as keys, so results[i] belongs to keys[i]. Duplicate keys are
	// looked up independently.
	GetMany(ctx context.Context, keys []string) ([]Result, error)

	// SetMany stores every item. When items contains the same key twice, the
	// last one wins. A nil error is not a guarantee that a subsequent GetMany
	// will hit.
	SetMany(ctx context.Context, items []Item) error
}
