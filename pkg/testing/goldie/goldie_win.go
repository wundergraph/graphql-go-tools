//go:build windows

package goldie

import (
	"bytes"
	"testing"
)

const IsWindows = true

func Assert(t *testing.T, name string, actual []byte, normalizeLineEndings ...bool) {
	if len(normalizeLineEndings) == 1 {
		actual = NormalizeNewlines(actual)
	}

	New(t).Assert(t, name, actual)
}

func Update(t *testing.T, name string, actual []byte) {
	panic("golden files should not be updated on windows")
}

func NormalizeNewlines(d []byte) []byte {
	// replace CR LF \r\n (windows) with LF \n (unix)
	d = bytes.Replace(d, []byte{13, 10}, []byte{10}, -1)
	// replace CF \r (mac) with LF \n (unix)
	d = bytes.Replace(d, []byte{13}, []byte{10}, -1)
	return d
}
