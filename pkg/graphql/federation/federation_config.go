package federation

import (
	"fmt"
	"net/http"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	graphqlDataSource "github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/federation"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

const (
	keyDirectiveName      = "key"
	requireDirectiveName  = "requires"
	externalDirectiveName = "external"
)

func NewEngineConfigV2Factory(httpClient *http.Client, dataSourceConfig ...graphqlDataSource.Configuration) *engineConfigV2Factory {
	return &engineConfigV2Factory{
		httpClient:        httpClient,
		dataSourceConfigs: dataSourceConfig,
	}
}

type engineConfigV2Factory struct {
	httpClient        *http.Client
	dataSourceConfigs []graphqlDataSource.Configuration
	schema            *graphql.Schema
}

func (f *engineConfigV2Factory) New() (*graphql.EngineV2Configuration, error) {
	var err error

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

	conf := graphql.NewEngineV2Configuration(f.schema)

	fieldConfigs, err := f.engineConfigFieldConfigs()
	if err != nil {
		return nil, fmt.Errorf("create field configs: %v", err)
	}

	datsSources, err := f.engineConfigDataSources()
	if err != nil {
		return nil, fmt.Errorf("create datasource config: %v", err)
	}

	conf.SetFieldConfigurations(fieldConfigs)
	conf.SetDataSources(datsSources)

	return &conf, nil
}

func (f *engineConfigV2Factory) engineConfigFieldConfigs() (plan.FieldConfigurations, error) {
	var planFieldConfigs plan.FieldConfigurations

	for _, dataSourceConfig := range f.dataSourceConfigs {
		doc, report := astparser.ParseGraphqlDocumentString(dataSourceConfig.Federation.ServiceSDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %w", report)
		}
		extractor := &requiredFieldExtractor{document: &doc}
		planFieldConfigs = append(planFieldConfigs, extractor.getAllFieldRequires()...)
	}

	generatedArgs := f.schema.GetAllFieldArguments(graphql.NewSkipReservedNamesFunc())
	generatedArgsAsLookupMap := graphql.CreateTypeFieldArgumentsLookupMap(generatedArgs)
	f.engineConfigArguments(&planFieldConfigs, generatedArgsAsLookupMap)

	return planFieldConfigs, nil
}

func (f *engineConfigV2Factory) engineConfigArguments(fieldConfs *plan.FieldConfigurations, generatedArgs map[graphql.TypeFieldLookupKey]graphql.TypeFieldArguments) {
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

func (f *engineConfigV2Factory) createArgumentConfigurationsForArgumentNames(argumentNames []string) plan.ArgumentsConfigurations {
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

func (f *engineConfigV2Factory) engineConfigDataSources() (planDataSources []plan.DataSourceConfiguration, err error) {
	for _, dataSourceConfig := range f.dataSourceConfigs {
		doc, report := astparser.ParseGraphqlDocumentString(dataSourceConfig.Federation.ServiceSDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %w", report)
		}

		var planDataSource plan.DataSourceConfiguration
		extractor := newNodeExtractor(&doc)
		planDataSource.RootNodes, planDataSource.ChildNodes = extractor.getAllNodes()

		factory := &graphqlDataSource.Factory{}
		if f.httpClient != nil {
			factory.Client = httpclient.NewNetHttpClient(f.httpClient)
		}
		planDataSource.Factory = factory

		planDataSource.Custom = graphqlDataSource.ConfigJson(dataSourceConfig)

		planDataSources = append(planDataSources, planDataSource)
	}

	return
}
