package execution

import (
	"encoding/json"
	"io"
)

type TypeDataSourcePlannerConfig struct {
}

type TypeDataSourcePlannerFactoryFactory struct {
}

func (t TypeDataSourcePlannerFactoryFactory) Initialize(base BaseDataSourcePlanner, configReader io.Reader) (DataSourcePlannerFactory, error) {
	factory := TypeDataSourcePlannerFactory{
		base: base,
	}
	return factory, json.NewDecoder(configReader).Decode(&factory.config)
}

type TypeDataSourcePlannerFactory struct {
	base   BaseDataSourcePlanner
	config TypeDataSourcePlannerConfig
}

func (t TypeDataSourcePlannerFactory) DataSourcePlanner() DataSourcePlanner {
	return SimpleDataSourcePlanner(&TypeDataSourcePlanner{
		BaseDataSourcePlanner: t.base,
		dataSourceConfig:      t.config,
	})
}

type TypeDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig TypeDataSourcePlannerConfig
}

func (t *TypeDataSourcePlanner) Plan(args []Argument) (DataSource, []Argument) {
	return &TypeDataSource{}, append(t.args,args...)
}

type TypeDataSource struct {
}

func (t *TypeDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	return CloseConnection
}
