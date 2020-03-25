package graphql

import (
	"io"
	"io/ioutil"

	"github.com/jensneuse/abstractlogger"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
)

type Schema struct {
	logger   abstractlogger.Logger
	document ast.Document
}

func NewSchemaFromReader(reader io.Reader) (*Schema, error) {
	logger := abstractlogger.NoopLogger

	schemaContent, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return createSchema(schemaContent, logger)
}

func NewSchemaFromString(schema string) (*Schema, error) {
	logger := abstractlogger.NoopLogger
	schemaContent := []byte(schema)

	return createSchema(schemaContent, logger)
}

func (s *Schema) Document() []byte {
	return s.document.Input.RawBytes
}

func (s *Schema) Validate() (valid bool, errors SchemaValidationErrors) {
	// TODO: Needs to be implemented in the core of the library
	return true, nil
}

func (s *Schema) SetLogger(logger abstractlogger.Logger) {
	s.logger = logger
}

func createSchema(schemaContent []byte, logger abstractlogger.Logger) (*Schema, error) {
	document, report := astparser.ParseGraphqlDocumentBytes(schemaContent)
	if report.HasErrors() {
		return nil, report
	}

	return &Schema{
		document: document,
		logger:   logger,
	}, nil
}
