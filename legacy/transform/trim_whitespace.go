package transform

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

// TrimWhitespace removes all spaces,tabs,lineterminators before and after a literal
func TrimWhitespace(lit []byte) []byte {
	for {

		if bytes.HasPrefix(lit, literal.SPACE) {
			lit = bytes.TrimPrefix(lit, literal.SPACE)
			continue
		}

		if bytes.HasPrefix(lit, literal.TAB) {
			lit = bytes.TrimPrefix(lit, literal.TAB)
			continue
		}

		if bytes.HasPrefix(lit, literal.LINETERMINATOR) {
			lit = bytes.TrimPrefix(lit, literal.LINETERMINATOR)
			continue
		}

		break
	}

	for {
		if bytes.HasSuffix(lit, literal.SPACE) {
			lit = bytes.TrimSuffix(lit, literal.SPACE)
			continue
		}

		if bytes.HasSuffix(lit, literal.TAB) {
			lit = bytes.TrimSuffix(lit, literal.TAB)
			continue
		}

		if bytes.HasSuffix(lit, literal.LINETERMINATOR) {
			lit = bytes.TrimSuffix(lit, literal.LINETERMINATOR)
			continue
		}

		break
	}

	return lit
}
