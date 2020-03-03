package execution

import (
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"io"
)

type SchemaDataSourcePlannerConfig struct {
}

type SchemaDataSourcePlannerFactoryFactory struct {
}

func (s SchemaDataSourcePlannerFactoryFactory) Initialize(base datasource.BasePlanner, configReader io.Reader) (datasource.PlannerFactory, error) {
	factory := &SchemaDataSourcePlannerFactory{
		base: base,
	}
	err := json.NewDecoder(configReader).Decode(&factory.config)
	if err != nil {
		return factory, err
	}
	gen := introspection.NewGenerator()
	var data introspection.Data
	var report operationreport.Report
	gen.Generate(base.definition, &report, &data)
	factory.schemaBytes, err = json.Marshal(data)
	return factory, err
}

type SchemaDataSourcePlannerFactory struct {
	base        datasource.BasePlanner
	config      SchemaDataSourcePlannerConfig
	schemaBytes []byte
}

func (s SchemaDataSourcePlannerFactory) DataSourcePlanner() datasource.Planner {
	return datasource.SimpleDataSourcePlanner(&SchemaDataSourcePlanner{
		BasePlanner:      s.base,
		dataSourceConfig: s.config,
		schemaBytes:      s.schemaBytes,
	})
}

type SchemaDataSourcePlanner struct {
	datasource.BasePlanner
	dataSourceConfig SchemaDataSourcePlannerConfig
	schemaBytes      []byte
}

func (s *SchemaDataSourcePlanner) Plan(args []Argument) (datasource.DataSource, []Argument) {
	return &SchemaDataSource{
		schemaBytes: s.schemaBytes,
	}, append(s.args, args...)
}

type SchemaDataSource struct {
	schemaBytes []byte
}

func (s *SchemaDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) (n int, err error) {
	return out.Write(s.schemaBytes)
}
