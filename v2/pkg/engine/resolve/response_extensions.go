package resolve

import "github.com/wundergraph/astjson"

// responseExtensionAccumulator owns the request-wide response-extension state
// for one deferred response. Callers append while holding the response's shared
// DataBuffer lock, which preserves the same merge order as response data and
// avoids a second synchronization primitive around arena-backed values.
//
// The accumulator is never shared across client requests. Values are backed by
// the request arena and remain alive until the deferred response completes.
type responseExtensionAccumulator struct {
	values              []*astjson.Object
	skipValueCompletion bool
}

func (a *responseExtensionAccumulator) append(value *astjson.Object) {
	if a == nil || value == nil {
		return
	}
	a.values = append(a.values, value)
}

// snapshot returns an immutable view of the collection order at this instant.
// The object values themselves are immutable after parsing; copying the slice
// prevents later deferred fetches from changing an earlier render's membership.
func (a *responseExtensionAccumulator) snapshot() []*astjson.Object {
	if a == nil || len(a.values) == 0 {
		return nil
	}
	return append([]*astjson.Object(nil), a.values...)
}

func (a *responseExtensionAccumulator) suppressValueCompletion() {
	if a != nil {
		a.skipValueCompletion = true
	}
}

func (a *responseExtensionAccumulator) valueCompletionSuppressed() bool {
	return a != nil && a.skipValueCompletion
}
