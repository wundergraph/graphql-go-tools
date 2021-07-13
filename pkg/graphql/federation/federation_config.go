package federation

import (
	"fmt"
	"net/http"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	graphqlDataSource "github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/federation"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

func NewEngineConfigV2Factory(
	httpClient *http.Client,
	batchFactory resolve.DataSourceBatchFactory,
	dataSourceConfig ...graphqlDataSource.Configuration,
) *EngineConfigV2Factory {
	return &EngineConfigV2Factory{
		httpClient:        httpClient,
		dataSourceConfigs: dataSourceConfig,
		batchFactory:      batchFactory,
	}
}

type EngineConfigV2Factory struct {
	httpClient        *http.Client
	dataSourceConfigs []graphqlDataSource.Configuration
	schema            *graphql.Schema
	batchFactory      resolve.DataSourceBatchFactory
}

func (f *EngineConfigV2Factory) SetMergedSchemaFromString(mergedSchema string) (err error) {
	f.schema, err = graphql.NewSchemaFromString(mergedSchema)
	if err != nil {
		return fmt.Errorf("set merged schema: %s", err.Error())
	}
	return nil
}

func (f *EngineConfigV2Factory) MergedSchema() (*graphql.Schema, error) {
	if f.schema != nil {
		return f.schema, nil
	}

	SDLs := make([]string, len(f.dataSourceConfigs))
	for i := range f.dataSourceConfigs {
		SDLs[i] = f.dataSourceConfigs[i].Federation.ServiceSDL
	}

	rawBaseSchema, err := federation.BuildBaseSchemaDocument(SDLs...)
	if err != nil {
		return nil, fmt.Errorf("build base schema: %v", err)
	}

	if f.schema, err = graphql.NewSchemaFromString(rawBaseSchema); err != nil {
		return nil, fmt.Errorf("parse schema from strinig: %v", err)
	}

	return f.schema, nil
}

func (f *EngineConfigV2Factory) EngineV2Configuration() (conf graphql.EngineV2Configuration, err error) {
	schema, err := f.MergedSchema()
	if err != nil {
		return conf, fmt.Errorf("get schema: %v", err)
	}

	conf = graphql.NewEngineV2Configuration(schema)

	fieldConfigs, err := f.engineConfigFieldConfigs(schema)
	if err != nil {
		return conf, fmt.Errorf("create field configs: %v", err)
	}

	datsSources, err := f.engineConfigDataSources()
	if err != nil {
		return conf, fmt.Errorf("create datasource config: %v", err)
	}

	conf.SetFieldConfigurations(fieldConfigs)
	conf.SetDataSources(datsSources)

	return conf, nil
}

func (f *EngineConfigV2Factory) engineConfigFieldConfigs(schema *graphql.Schema) (plan.FieldConfigurations, error) {
	var planFieldConfigs plan.FieldConfigurations

	for _, dataSourceConfig := range f.dataSourceConfigs {
		doc, report := astparser.ParseGraphqlDocumentString(dataSourceConfig.Federation.ServiceSDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %s", report.Error())
		}
		extractor := plan.NewRequiredFieldExtractor(&doc)
		planFieldConfigs = append(planFieldConfigs, extractor.GetAllRequiredFields()...)
	}

	generatedArgs := schema.GetAllFieldArguments(graphql.NewSkipReservedNamesFunc())
	generatedArgsAsLookupMap := graphql.CreateTypeFieldArgumentsLookupMap(generatedArgs)
	f.engineConfigArguments(&planFieldConfigs, generatedArgsAsLookupMap)

	return planFieldConfigs, nil
}

func (f *EngineConfigV2Factory) engineConfigArguments(fieldConfs *plan.FieldConfigurations, generatedArgs map[graphql.TypeFieldLookupKey]graphql.TypeFieldArguments) {
	for i := range *fieldConfs {
		if len(generatedArgs) == 0 {
			return
		}

		lookupKey := graphql.CreateTypeFieldLookupKey((*fieldConfs)[i].TypeName, (*fieldConfs)[i].FieldName)
		currentArgs, exists := generatedArgs[lookupKey]
		if !exists {
			continue
		}

		(*fieldConfs)[i].Arguments = f.createArgumentConfigurationsForArgumentNames(currentArgs.ArgumentNames)
		delete(generatedArgs, lookupKey)
	}

	for _, genArgs := range generatedArgs {
		*fieldConfs = append(*fieldConfs, plan.FieldConfiguration{
			TypeName:  genArgs.TypeName,
			FieldName: genArgs.FieldName,
			Arguments: f.createArgumentConfigurationsForArgumentNames(genArgs.ArgumentNames),
		})
	}
}

func (f *EngineConfigV2Factory) createArgumentConfigurationsForArgumentNames(argumentNames []string) plan.ArgumentsConfigurations {
	argConfs := plan.ArgumentsConfigurations{}
	for _, argName := range argumentNames {
		argConf := plan.ArgumentConfiguration{
			Name:       argName,
			SourceType: plan.FieldArgumentSource,
		}

		argConfs = append(argConfs, argConf)
	}

	return argConfs
}

func (f *EngineConfigV2Factory) engineConfigDataSources() (planDataSources []plan.DataSourceConfiguration, err error) {
	for _, dataSourceConfig := range f.dataSourceConfigs {
		doc, report := astparser.ParseGraphqlDocumentString(dataSourceConfig.Federation.ServiceSDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %s", report.Error())
		}

		var planDataSource plan.DataSourceConfiguration
		extractor := plan.NewLocalTypeFieldExtractor(&doc)
		planDataSource.RootNodes, planDataSource.ChildNodes = extractor.GetAllNodes()

		factory := &graphqlDataSource.Factory{
			BatchFactory: f.batchFactory,
		}
		if f.httpClient != nil {
			factory.Client = httpclient.NewNetHttpClient(f.httpClient)
		}
		planDataSource.Factory = factory

		planDataSource.Custom = graphqlDataSource.ConfigJson(dataSourceConfig)

		planDataSources = append(planDataSources, planDataSource)
	}

	return
}
