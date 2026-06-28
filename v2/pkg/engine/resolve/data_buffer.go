package resolve

import (
	"sync"

	"github.com/wundergraph/astjson"
)

// DataBuffer holds the shared response JSON tree — the accumulated document that
// every fetch merges into and every render reads — together with the mutex that
// guards it.
//
// Locking contract (advisory — the accessors do NOT enforce it): a caller MUST
// hold Lock() for the entire region in which it reads or mutates the tree. That
// includes mutating the *astjson.Value returned by Get (merges happen in place)
// and swapping it via Set; Get and Set do not take the lock themselves.
//
// The guarded region is compound and spans components, which is why Lock/Unlock
// are exposed instead of being wrapped in a single accessor: during deferred
// resolution the Loader merges a group's fetched data under the lock
// (Loader.mergeResult), and resolve.go keeps the same lock held across the
// following render and flush so concurrent defer groups cannot interleave their
// frames. Only the Loader holds a *DataBuffer; resolve.go reads via Get (under
// the lock) and injects the value into Resolvable.data before each render —
// Resolvable never references the DataBuffer.
type DataBuffer struct {
	mu   sync.Mutex
	data *astjson.Value
}

// Lock acquires the guard. It must be held for the whole critical section that
// reads or mutates the tree (merge, render, and flush during deferred delivery),
// not merely around a single Get or Set call.
func (d *DataBuffer) Lock() {
	d.mu.Lock()
}

// Unlock releases the guard acquired by Lock.
func (d *DataBuffer) Unlock() {
	d.mu.Unlock()
}

// Get returns the current data value. The caller must hold Lock, and must read
// or mutate the returned value only while still holding it.
func (d *DataBuffer) Get() *astjson.Value { return d.data }

// Set replaces the data value (the root-level merge case in Loader.mergeResult).
// The caller must hold Lock.
func (d *DataBuffer) Set(v *astjson.Value) { d.data = v }
