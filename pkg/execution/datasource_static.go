package execution

import (
	"encoding/json"
	"io"
)

type StaticDataSourceConfig struct {
	Data string
}

type StaticDataSourcePlannerFactoryFactory struct {
}

func (s StaticDataSourcePlannerFactoryFactory) Initialize(base BaseDataSourcePlanner, configReader io.Reader) (DataSourcePlannerFactory, error) {
	factory := &StaticDataSourcePlannerFactory{
		base: base,
	}
	return factory, json.NewDecoder(configReader).Decode(&factory.config)
}

type StaticDataSourcePlannerFactory struct {
	base   BaseDataSourcePlanner
	config StaticDataSourceConfig
}

func (s StaticDataSourcePlannerFactory) DataSourcePlanner() DataSourcePlanner {
	return SimpleDataSourcePlanner(&StaticDataSourcePlanner{
		BaseDataSourcePlanner: s.base,
		dataSourceConfig:      s.config,
	})
}

type StaticDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig StaticDataSourceConfig
}

func (s *StaticDataSourcePlanner) Plan(args []Argument) (DataSource, []Argument) {
	return &StaticDataSource{
		data: []byte(s.dataSourceConfig.Data),
	}, append(s.args,args...)
}

type StaticDataSource struct {
	data []byte
}

func (s StaticDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	_, _ = out.Write(s.data)
	return CloseConnectionIfNotStream
}
