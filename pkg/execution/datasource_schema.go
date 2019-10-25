package execution

import (
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func NewSchemaDataSourcePlanner(definition *ast.Document, report *operationreport.Report) *SchemaDataSourcePlanner {
	gen := introspection.NewGenerator()
	var data introspection.Data
	gen.Generate(definition, report, &data)
	schemaBytes, err := json.Marshal(data)
	if err != nil {
		report.AddInternalError(err)
	}
	return &SchemaDataSourcePlanner{
		schemaBytes: schemaBytes,
	}
}

type SchemaDataSourcePlanner struct {
	schemaBytes []byte
}

func (s *SchemaDataSourcePlanner) DirectiveName() []byte {
	return []byte("resolveSchema")
}

func (s *SchemaDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (s *SchemaDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (s *SchemaDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (s *SchemaDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (s *SchemaDataSourcePlanner) EnterField(ref int) {

}

func (s *SchemaDataSourcePlanner) LeaveField(ref int) {

}

func (s *SchemaDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &SchemaDataSource{
		schemaBytes: s.schemaBytes,
	}, nil
}

type SchemaDataSource struct {
	schemaBytes []byte
}

func (s *SchemaDataSource) Resolve(ctx Context, args ResolvedArgs) []byte {
	return s.schemaBytes
}
