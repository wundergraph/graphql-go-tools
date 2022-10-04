//go:build (darwin && cgo) || linux

package goldie

import (
	"testing"
)

func Assert(t *testing.T, name string, actual []byte, _ ...bool) {
	New(t).Assert(t, name, actual)
}

func Update(t *testing.T, name string, actual []byte) {
	_ = New(t).Update(t, name, actual)
}
