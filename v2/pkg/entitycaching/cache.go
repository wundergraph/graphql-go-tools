package entitycaching

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Item is a single cache entry, both on the way in through SetMany and on the
// way back out of GetMany.
type Item struct {
	// Key is the key the entry is stored under, exactly as GetMany was asked
	// for it, so an Item carries enough to be acted on once it has been taken
	// out of the map it came in.
	Key string
	// Value is the cached bytes. An entry with an empty Value is still an
	// entry: GetMany omitting the key is the only thing that means a miss.
	Value []byte
	// TTL is how long the entry should live when passed to SetMany, and how
	// much of that life it has left when returned by GetMany, so the same key
	// comes back with less of it on every lookup. It is always positive in
	// both directions: SetMany refuses an item without a positive TTL, and
	// GetMany treats an entry with nothing left to live, or one it finds with
	// no expiry attached at all, as a miss rather than a hit.
	TTL time.Duration
}

// Cache is a batch oriented key/value cache.
type Cache interface {
	// GetMany looks up every key and returns the ones it found, keyed by the
	// key they were asked for. A miss is simply absent, never an error and
	// never a zero Item
	GetMany(ctx context.Context, keys []string) (map[string]Item, error)

	// SetMany stores every item, all of which must carry a positive TTL. When
	// items contains the same key twice, the last one wins. An error means an
	// unspecified subset of the items may already have been stored.
	// In case any of the passed ttls are invalid, SetMany should return
	// without saving any items that may have valid ttl values
	SetMany(ctx context.Context, items []Item) error
}

var ErrMissingTTL = errors.New("cache item requires a positive TTL")

type SetManyError struct {
	// This depicts the known stored keys in a case of a partial write
	// this means that some more keys could have been written but there is no way to know
	// because the client did not receive a response
	KnownStoredKeys []string
	Err             error
}

func (e *SetManyError) Error() string {
	return fmt.Sprintf("%v (%d keys were known to be written)", e.Err, len(e.KnownStoredKeys))
}

func (e *SetManyError) Unwrap() error {
	return e.Err
}
