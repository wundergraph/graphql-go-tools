package loader

import (
	"encoding/json"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/rest_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/staticdatasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

type Loader struct {
	resolvers []FactoryResolver
}

type FactoryResolver interface {
	Resolve(dataSourceName string) plan.PlannerFactory
}

type DefaultFactoryResolver struct {
	graphql *graphql_datasource.Factory
	rest    *rest_datasource.Factory
	static  *staticdatasource.Factory
}

func NewDefaultFactoryResolver(client httpclient.Client) *DefaultFactoryResolver {
	return &DefaultFactoryResolver{
		graphql: &graphql_datasource.Factory{
			Client: client,
		},
		rest: &rest_datasource.Factory{
			Client: client,
		},
		static: &staticdatasource.Factory{},
	}
}

func (d *DefaultFactoryResolver) Resolve(dataSourceName string) plan.PlannerFactory {
	switch dataSourceName {
	case graphql_datasource.UniqueIdentifier:
		return d.graphql
	case rest_datasource.UniqueIdentifier:
		return d.rest
	case staticdatasource.UniqueIdentifier:
		return d.static
	default:
		return nil
	}
}

func New(resolvers ...FactoryResolver) *Loader {
	return &Loader{
		resolvers: resolvers,
	}
}

func (l *Loader) Load(data []byte) (plan.Configuration, error) {
	var (
		inConfig  Configuration
		outConfig plan.Configuration
	)
	err := json.Unmarshal(data, &inConfig)
	if err != nil {
		return outConfig, err
	}

	outConfig.Schema = inConfig.Schema
	outConfig.DefaultFlushInterval = inConfig.DefaultFlushInterval
	outConfig.Fields = inConfig.Fields

	for _, in := range inConfig.DataSources {
		factory := l.resolveFactory(in.DataSourceName)
		if factory == nil {
			continue
		}
		out := plan.DataSourceConfiguration{
			Factory:                    factory,
			Custom:                     uglifyJSON(in.Custom),
			RootNodes:                  in.RootNodes,
			ChildNodes:                 in.ChildNodes,
			OverrideFieldPathFromAlias: in.OverrideFieldPathFromAlias,
		}
		outConfig.DataSources = append(outConfig.DataSources, out)
	}

	return outConfig, nil
}

func (l *Loader) resolveFactory(dataSourceName string) plan.PlannerFactory {
	for i := range l.resolvers {
		factory := l.resolvers[i].Resolve(dataSourceName)
		if factory != nil {
			return factory
		}
	}
	return nil
}

func uglifyJSON(in json.RawMessage) json.RawMessage {
	var input interface{}
	_ = json.Unmarshal(in, &input)
	out, _ := json.Marshal(input)
	return out
}

type Configuration struct {
	DefaultFlushInterval int64
	DataSources          []DataSourceConfiguration
	Fields               []plan.FieldConfiguration
	Schema               string
}

type DataSourceConfiguration struct {
	plan.DataSourceConfiguration
	DataSourceName string
}
