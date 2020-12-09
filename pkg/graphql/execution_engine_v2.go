package graphql

import (
	"sync"

	"github.com/jensneuse/abstractlogger"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

type EngineV2Configuration struct {
	schema        *Schema
	plannerConfig plan.Configuration
}

func NewEngineV2Configuration(schema *Schema) EngineV2Configuration {
	return EngineV2Configuration{
		schema: schema,
		plannerConfig: plan.Configuration{
			DefaultFlushInterval: 0,
			DataSources:          []plan.DataSourceConfiguration{},
			Fields:               plan.FieldConfigurations{},
			Schema:               string(schema.rawInput),
		},
	}
}

func (e *EngineV2Configuration) AddDataSource(dataSource plan.DataSourceConfiguration) {
	e.plannerConfig.DataSources = append(e.plannerConfig.DataSources, dataSource)
}

func (e *EngineV2Configuration) SetDataSources(dataSources []plan.DataSourceConfiguration) {
	e.plannerConfig.DataSources = dataSources
}

func (e *EngineV2Configuration) AddFieldConfiguration(fieldConfig plan.FieldConfiguration) {
	e.plannerConfig.Fields = append(e.plannerConfig.Fields, fieldConfig)
}

func (e *EngineV2Configuration) SetFieldConfiguration(fieldConfigs plan.FieldConfigurations) {
	e.plannerConfig.Fields = fieldConfigs
}

type ExecutionEngineV2 struct {
	logger      abstractlogger.Logger
	config      EngineV2Configuration
	planner     *plan.Planner
	resolver    *resolve.Resolver
	contextPool sync.Pool
}

func NewExecutionEngineV2(logger abstractlogger.Logger, engineConfig EngineV2Configuration) (*ExecutionEngineV2, error) {
	return &ExecutionEngineV2{
		logger:   logger,
		config:   engineConfig,
		planner:  plan.NewPlanner(engineConfig.plannerConfig),
		resolver: resolve.New(),
		contextPool: sync.Pool{
			New: func() interface{} {
				return resolve.NewContext(nil)
			},
		},
	}, nil
}
