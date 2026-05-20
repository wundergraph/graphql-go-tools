// Package variables provides a request-scoped view over GraphQL operation
// variables that transparently honors variable remapping introduced during
// operation normalization.
package variables

import "github.com/wundergraph/astjson"

// Set is a read-side view of request variables.
//
// The zero value is valid and behaves as an empty set: Get returns nil for any
// name. Set is intended to be passed by value; it carries only two
// pointers and is safe to copy.
type Set struct {
	variables *astjson.Value
	remap     map[string]string
}

// NewSet returns a Set that reads variable values from vars,
// using remap (new -> old name) to translate post-normalization variable names
// back to the original keys present in vars.
// Either argument may be nil; a nil remap means no translation is performed.
func NewSet(vars *astjson.Value, remap map[string]string) Set {
	return Set{variables: vars, remap: remap}
}

// Get walks the variable tree along path. path[0] is the variable name and
// is translated through the remap if an entry exists.
// Subsequent elements are walked as nested keys on the resulting JSON value.
// Returns nil if the set is empty, the path is empty, or any segment is missing.
func (v Set) Get(path ...string) *astjson.Value {
	if v.variables == nil || len(path) == 0 {
		return nil
	}
	head := path[0]
	if orig, ok := v.remap[head]; ok {
		head = orig
	}
	val := v.variables.Get(head)
	if val == nil || len(path) == 1 {
		return val
	}
	return val.Get(path[1:]...)
}

func (v Set) IsEmpty() bool {
	return v.variables == nil
}
