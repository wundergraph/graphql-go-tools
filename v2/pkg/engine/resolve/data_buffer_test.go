package resolve

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/astjson"
)

func TestDataBuffer_LockDisabled(t *testing.T) {
	d := &DataBuffer{}
	// Must not panic or deadlock when enableLock is false.
	d.Lock()
	d.Unlock()
}

func TestDataBuffer_LockEnabled(t *testing.T) {
	// Run with `go test -race` to have the race detector verify serialisation.
	// Two goroutines each write a distinct field on the shared object under the lock;
	// afterwards the object must contain both fields.
	obj := astjson.ObjectValue(nil)
	d := &DataBuffer{enableLock: true, data: obj}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		d.Lock()
		astjson.SetValue(nil, d.Get(), astjson.StringValue(nil, "a"), "a")
		d.Unlock()
	}()
	go func() {
		defer wg.Done()
		d.Lock()
		astjson.SetValue(nil, d.Get(), astjson.StringValue(nil, "b"), "b")
		d.Unlock()
	}()
	wg.Wait()

	got := d.Get().String()
	assert.Contains(t, got, `"a":"a"`, "field a must be present")
	assert.Contains(t, got, `"b":"b"`, "field b must be present")
}

func TestDataBuffer_GetSet(t *testing.T) {
	d := &DataBuffer{}
	assert.Nil(t, d.Get())
	// Set accepts nil without panic.
	d.Set(nil)
	assert.Nil(t, d.Get())
}
