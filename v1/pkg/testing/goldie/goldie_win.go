//go:build windows

package goldie

import (
	"testing"
)

func Assert(t *testing.T, name string, actual []byte, useOSSuffix ...bool) {
	t.Helper()

	if len(useOSSuffix) == 1 && useOSSuffix[0] {
		name = name + "_windows"
	}

	New(t).Assert(t, name, actual)
}

func Update(t *testing.T, name string, actual []byte) {
	t.Helper()
	t.Fatalf("golden files should not be updated on windows")
}
