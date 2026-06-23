package resolve

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
)

func TestDataBuffer_LockEnabled(t *testing.T) {
	// Run with `go test -race` to have the race detector verify serialisation.
	// Two goroutines each write a distinct field on the shared object under the lock;
	// afterwards the object must contain both fields.
	obj := astjson.ObjectValue(nil)
	d := &DataBuffer{data: obj}

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
