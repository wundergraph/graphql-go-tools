package ast

import (
	"io"
	"strings"

	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/position"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/runes"
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
	skipWhitespace := false
	skippedWhitespace := 0.0
	depthToSkip := float64(depth)
	for i := range content {

		if skipWhitespace && skippedWhitespace < depthToSkip {
			switch content[i] {
			case runes.TAB:
				skippedWhitespace += 1
				continue
			case runes.SPACE:
				skippedWhitespace += 0.5
				continue
			}
		}

		switch content[i] {
		case runes.LINETERMINATOR:
			skipWhitespace = true
		default:
			if skipWhitespace {
				for j := 0; j < depth; j++ {
					_, err = writer.Write(indent)
				}

				skipWhitespace = false
				skippedWhitespace = 0.0
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
