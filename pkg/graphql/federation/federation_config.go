package federation

import (
	"fmt"
	"net/http"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

const (
	keyDirectiveName      = "key"
	requireDirectiveName  = "requires"
	externalDirectiveName = "external"
)

func NewEngineConfigV2Factory(httpClient *http.Client, rawBaseSchema string, SDLs ...string) *engineConfigV2Factory {
	return &engineConfigV2Factory{
		httpClient:    httpClient,
		rawBaseSchema: rawBaseSchema,
		SDLs:          SDLs,
	}
}

type engineConfigV2Factory struct {
	rawBaseSchema string
	schema        *graphql.Schema
	httpClient    *http.Client
	SDLs          []string
}

func (f *engineConfigV2Factory) New() (*graphql.EngineV2Configuration, error) {
	var err error
	if f.schema, err = graphql.NewSchemaFromString(f.rawBaseSchema); err != nil {
		return nil, err
	}

	conf := graphql.NewEngineV2Configuration(f.schema)

	fieldConfigs, err := f.engineConfigFieldConfigs()
	if err != nil {
		return nil, err
	}

	conf.SetFieldConfigurations(fieldConfigs)

	return nil, nil
}

func (f *engineConfigV2Factory) engineConfigFieldConfigs() (plan.FieldConfigurations, error) {
	var planFieldConfigs plan.FieldConfigurations

	for _, SDL := range f.SDLs {
		doc, report := astparser.ParseGraphqlDocumentString(SDL)
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

//
