package resolve

import (
	"sync"

	"github.com/wundergraph/astjson"
)

// DataBuffer holds the shared response JSON tree and its concurrency guard.
//
// enableLock is false during normal single-threaded execution (no locking
// overhead) and set to true before parallel defer groups are launched.
//
// Only the Loader holds a *DataBuffer. The Loader reads/writes the tree during
// the fetch phase; resolve.go reads via Get and injects the value into
// Resolvable.data before each render. Resolvable never references the DataBuffer.
type DataBuffer struct {
	mu         sync.Mutex
	enableLock bool
	data       *astjson.Value
}

// Lock acquires the mutex when parallel execution is active.
func (d *DataBuffer) Lock() {
	if d.enableLock {
		d.mu.Lock()
	}
}

// Unlock releases the mutex when parallel execution is active.
func (d *DataBuffer) Unlock() {
	if d.enableLock {
		d.mu.Unlock()
	}
}

// Get returns the current data value.
func (d *DataBuffer) Get() *astjson.Value { return d.data }

// Set replaces the data value (root-level merge case in Loader.mergeResult).
func (d *DataBuffer) Set(v *astjson.Value) { d.data = v }
