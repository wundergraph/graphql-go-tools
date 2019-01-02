package transform

import (
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

// TrimWhitespace removes all spaces,tabs,lineterminators before and after a literal
func TrimWhitespace(lit string) string {
	for {

		if strings.HasPrefix(lit, literal.SPACE) {
			lit = strings.TrimPrefix(lit, literal.SPACE)
			continue
		}

		if strings.HasPrefix(lit, literal.TAB) {
			lit = strings.TrimPrefix(lit, literal.TAB)
			continue
		}

		if strings.HasPrefix(lit, literal.LINETERMINATOR) {
			lit = strings.TrimPrefix(lit, literal.LINETERMINATOR)
			continue
		}

		break
	}

	for {
		if strings.HasSuffix(lit, literal.SPACE) {
			lit = strings.TrimSuffix(lit, literal.SPACE)
			continue
		}

		if strings.HasSuffix(lit, literal.TAB) {
			lit = strings.TrimSuffix(lit, literal.TAB)
			continue
		}

		if strings.HasSuffix(lit, literal.LINETERMINATOR) {
			lit = strings.TrimSuffix(lit, literal.LINETERMINATOR)
			continue
		}

		break
	}

	return lit
}
