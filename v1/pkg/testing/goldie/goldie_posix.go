//go:build !windows

package goldie

import (
	"testing"
)

func Assert(t *testing.T, name string, actual []byte, _ ...bool) {
	t.Helper()

	New(t).Assert(t, name, actual)
}

func Update(t *testing.T, name string, actual []byte) {
	t.Helper()

	_ = New(t).Update(t, name, actual)
}
