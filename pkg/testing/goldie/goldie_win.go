//go:build windows

package goldie

import (
	"testing"
)

func Assert(t *testing.T, name string, actual []byte) {
	t.Log("skipping goldie assertion on windows")
}

func Update(t *testing.T, name string, actual []byte) {
	panic("golden files should not be updated on windows")
}
