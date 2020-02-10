package imports

import (
	"bufio"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
	"os"
)

type GraphQLFile struct {
	RelativePath string
	Imports      []GraphQLFile
}

func (g GraphQLFile) Render(printFilePath bool, out io.Writer) error {
	return g.render(printFilePath, out)
}

func (g GraphQLFile) render(printFilePath bool, out io.Writer) error {

	var err error
	if g.RelativePath != "" {
		err = g.renderSelf(printFilePath, out)
		if err != nil {
			return err
		}
	}

	for _, importFile := range g.Imports {
		if printFilePath {
			_, err = out.Write(literal.LINETERMINATOR)
			if err != nil {
				return err
			}
			_, err = out.Write(literal.LINETERMINATOR)
			if err != nil {
				return err
			}
		}
		err = importFile.render(printFilePath, out)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g GraphQLFile) renderSelf(printFilePath bool, out io.Writer) error {
	file, err := os.Open(g.RelativePath)
	if err != nil {
		return err
	}
	defer file.Close()

	if printFilePath {
		err = g.renderFilePath(out)
		if err != nil {
			return err
		}
	}

	reader := bufio.NewReader(file)
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			break
		}

		if importStatementRegex.Match(line) {
			continue
		}

		_, err = out.Write(line)
		if err != nil {
			return err
		}

		_, err = out.Write(literal.LINETERMINATOR)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g GraphQLFile) renderFilePath(out io.Writer) error {
	_, err := out.Write([]byte("#file: " + g.RelativePath + "\n\n"))
	return err
}
