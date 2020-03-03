package execution

import (
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"io"
)

type TypeDataSourcePlannerConfig struct {
}

type TypeDataSourcePlannerFactoryFactory struct {
}

func (t TypeDataSourcePlannerFactoryFactory) Initialize(base datasource.BasePlanner, configReader io.Reader) (datasource.PlannerFactory, error) {
	factory := TypeDataSourcePlannerFactory{
		base: base,
	}
	return factory, json.NewDecoder(configReader).Decode(&factory.config)
}

type TypeDataSourcePlannerFactory struct {
	base   datasource.BasePlanner
	config TypeDataSourcePlannerConfig
}

func (t TypeDataSourcePlannerFactory) DataSourcePlanner() datasource.Planner {
	return datasource.SimpleDataSourcePlanner(&TypeDataSourcePlanner{
		BasePlanner:      t.base,
		dataSourceConfig: t.config,
	})
}

type TypeDataSourcePlanner struct {
	datasource.BasePlanner
	dataSourceConfig TypeDataSourcePlannerConfig
}

func (t *TypeDataSourcePlanner) Plan(args []Argument) (datasource.DataSource, []Argument) {
	return &TypeDataSource{}, append(t.args, args...)
}

type TypeDataSource struct {
}

func (t *TypeDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) (n int, err error) {
	return
}
