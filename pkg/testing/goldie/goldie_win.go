//go:build windows

package goldie

import (
	"bytes"
	"testing"
)

func Assert(t *testing.T, name string, actual []byte) {
	New(t).Assert(t, name, bytes.ReplaceAll(actual, []byte("\r\n"), []byte("\n")))
}

func Update(t *testing.T, name string, actual []byte) {
	_ = New(t).Update(t, name, actual)
}
