package graphql

import (
	"io"
	"io/ioutil"

	"github.com/jensneuse/abstractlogger"

	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
)

type Schema struct {
	logger      abstractlogger.Logger
	document    []byte
	basePlanner *datasource.BasePlanner
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
	return s.document
}

func (s *Schema) Validate() (valid bool, errors SchemaValidationErrors) {
	// TODO: Needs to be implemented in the core of the library
	return true, nil
}

func (s *Schema) AddLogger(logger abstractlogger.Logger) {
	s.logger = logger
	s.basePlanner.Log = logger
}

func createSchema(document []byte, logger abstractlogger.Logger) (*Schema, error) {
	basePlanner, err := datasource.NewBaseDataSourcePlanner(document, datasource.PlannerConfiguration{}, logger)
	if err != nil {
		return nil, err
	}

	return &Schema{
		document:    document,
		logger:      logger,
		basePlanner: basePlanner,
	}, nil
}
