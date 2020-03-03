package execution

import (
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"io"
)

type StaticDataSourceConfig struct {
	Data string
}

type StaticDataSourcePlannerFactoryFactory struct {
}

func (s StaticDataSourcePlannerFactoryFactory) Initialize(base datasource.BasePlanner, configReader io.Reader) (datasource.PlannerFactory, error) {
	factory := &StaticDataSourcePlannerFactory{
		base: base,
	}
	return factory, json.NewDecoder(configReader).Decode(&factory.config)
}

type StaticDataSourcePlannerFactory struct {
	base   datasource.BasePlanner
	config StaticDataSourceConfig
}

func (s StaticDataSourcePlannerFactory) DataSourcePlanner() datasource.Planner {
	return datasource.SimpleDataSourcePlanner(&StaticDataSourcePlanner{
		BasePlanner:      s.base,
		dataSourceConfig: s.config,
	})
}

type StaticDataSourcePlanner struct {
	datasource.BasePlanner
	dataSourceConfig StaticDataSourceConfig
}

func (s *StaticDataSourcePlanner) Plan(args []Argument) (datasource.DataSource, []Argument) {
	return &StaticDataSource{
		data: []byte(s.dataSourceConfig.Data),
	}, append(s.args, args...)
}

type StaticDataSource struct {
	data []byte
}

func (s StaticDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) (n int, err error) {
	return out.Write(s.data)
}
