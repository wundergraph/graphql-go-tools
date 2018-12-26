package diffview

import (
	"os"
	"os/exec"
)

type golandDiffViewer struct {
}

func (golandDiffViewer) open(a, b string) error {
	cmd := exec.Command("/Applications/GoLand.app/Contents/MacOS/goland", "diff", a, b)
	cmd.Env = os.Environ()
	return cmd.Run()
}
