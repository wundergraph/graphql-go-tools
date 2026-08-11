package resolve

import (
	"github.com/pkg/errors"

	arena "github.com/wundergraph/go-arena"
)

// batchInputAssembly holds the pre-rendered pieces of a batch entity input:
// header/separator/footer plus one segment per UNIQUE representation (bucket
// order). All slices live on the fetch's batchEntityTools arena, which stays
// alive until resolveSingle returns.
//
// It exists to defer the final input rendering until AFTER the cache decision:
// assemble runs exactly once, producing an input that contains only the
// representations the network fetch still needs (no parse-back filtering).
type batchInputAssembly struct {
	header             []byte
	separator          []byte
	footer             []byte
	segments           [][]byte
	undefinedVariables []string
}

// assemble renders the FINAL batch input into a fresh buffer on a: the header,
// the kept segments separator-joined (nil keep = all; a segment index beyond
// keep's length is dropped), the footer — then the undefined-variables
// post-processing on the final bytes.
func (b *batchInputAssembly) assemble(a arena.Arena, keep []bool) ([]byte, error) {
	buf := arena.NewArenaBuffer(a)
	_, _ = buf.Write(b.header)
	wroteItem := false
	for i, segment := range b.segments {
		if keep != nil && (i >= len(keep) || !keep[i]) {
			continue
		}
		if wroteItem {
			_, _ = buf.Write(b.separator)
		}
		_, _ = buf.Write(segment)
		wroteItem = true
	}
	_, _ = buf.Write(b.footer)
	if err := SetInputUndefinedVariables(buf, b.undefinedVariables); err != nil {
		return nil, errors.WithStack(err)
	}
	return buf.Bytes(), nil
}
