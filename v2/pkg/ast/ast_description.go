package ast

import (
	"io"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/runes"
)

type Description struct {
	IsDefined     bool
	IsBlockString bool               // true if -> """content""" ; else "content"
	Content       ByteSliceReference // literal
	Position      position.Position
}

// nolint
func (d *Document) PrintDescription(description Description, indent []byte, depth int, writer io.Writer) (err error) {
	for i := 0; i < depth; i++ {
		_, err = writer.Write(indent)
	}
	if description.IsBlockString {
		_, err = writer.Write(literal.QUOTE)
		_, err = writer.Write(literal.QUOTE)
		_, err = writer.Write(literal.QUOTE)
		_, err = writer.Write(literal.LINETERMINATOR)
		for i := 0; i < depth; i++ {
			_, err = writer.Write(indent)
		}
	} else {
		_, err = writer.Write(literal.QUOTE)
	}

	content := d.Input.ByteSlice(description.Content)

	// The lexer preserves the source-level indentation on every line after the
	// first, so before re-emitting we strip the common indent of lines 1+
	// (per the BlockStringValue() canonicalization in the GraphQL spec). The
	// per-line depth prefix is then added back below. This preserves any
	// deliberate inner indentation — e.g. code blocks inside a description.
	commonIndent := commonBlockStringIndent(splitBytesIntoLines(content))
	if commonIndent < 0 {
		commonIndent = 0
	}

	skipWhitespace := false
	skippedBytes := 0
	for i := range content {
		if skipWhitespace && skippedBytes < commonIndent {
			switch content[i] {
			case runes.TAB, runes.SPACE:
				skippedBytes++
				continue
			}
		}

		switch content[i] {
		case runes.LINETERMINATOR:
			skipWhitespace = true
			skippedBytes = 0
		default:
			if skipWhitespace {
				for j := 0; j < depth; j++ {
					_, err = writer.Write(indent)
				}
				skipWhitespace = false
			}
		}
		_, err = writer.Write(content[i : i+1])
	}
	if description.IsBlockString {
		_, err = writer.Write(literal.LINETERMINATOR)
		for i := 0; i < depth; i++ {
			_, err = writer.Write(indent)
		}
		_, err = writer.Write(literal.QUOTE)
		_, err = writer.Write(literal.QUOTE)
		_, err = writer.Write(literal.QUOTE)
	} else {
		_, err = writer.Write(literal.QUOTE)
	}
	return nil
}

func (d *Document) ImportDescription(desc string) (description Description) {
	if desc == "" {
		return
	}

	isBlockString := strings.Contains(desc, "\n") || strings.Contains(desc, `"`)

	return Description{
		IsDefined:     true,
		IsBlockString: isBlockString,
		Content:       d.Input.AppendInputString(desc),
	}
}
