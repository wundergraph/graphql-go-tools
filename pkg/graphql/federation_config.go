package graphql

import (
	"fmt"
	"net/http"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/federation/planconfiguration"
)

func NewFederationEngineConfigV2Factory(httpClient *http.Client, rawBaseSchema string, SDLs ...string) *FederationEngineConfigV2Factory {
	return &FederationEngineConfigV2Factory{
		httpClient:    httpClient,
		rawBaseSchema: rawBaseSchema,
		SDLs:          SDLs,
	}
}

type FederationEngineConfigV2Factory struct {
	rawBaseSchema string
	schema        *Schema
	httpClient    *http.Client
	SDLs          []string
}

func (f *FederationEngineConfigV2Factory) New() (*EngineV2Configuration, error) {
	var err error
	if f.schema, err = NewSchemaFromString(f.rawBaseSchema); err != nil {
		return nil, err
	}

	conf := NewEngineV2Configuration(f.schema)

	fieldConfigs, err := f.engineConfigFieldConfigs()
	if err != nil {
		return nil, err
	}

	conf.SetFieldConfigurations(fieldConfigs)

	return nil, nil
}

func (f *FederationEngineConfigV2Factory) engineConfigFieldConfigs() (plan.FieldConfigurations, error) {
	var typeFieldRequires []planconfiguration.TypeFieldRequires

	for _, SDL := range f.SDLs {
		doc, report := astparser.ParseGraphqlDocumentString(SDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %w", report)
		}
		typeFieldRequires = append(typeFieldRequires, planconfiguration.ExtractRequiredFields(&doc)...)
	}

	planFieldConfigs := make(plan.FieldConfigurations, len(typeFieldRequires))
	for i := range typeFieldRequires {
		planFieldConfigs[i] = plan.FieldConfiguration{
			TypeName:       typeFieldRequires[i].TypeName,
			FieldName:      typeFieldRequires[i].FieldName,
			RequiresFields: typeFieldRequires[i].RequiresFields,
		}
	}

	generatedArgs := f.schema.GetAllFieldArguments(NewSkipReservedNamesFunc())
	generatedArgsAsLookupMap := CreateTypeFieldArgumentsLookupMap(generatedArgs)
	f.engineConfigArguments(&planFieldConfigs, generatedArgsAsLookupMap)

	return planFieldConfigs, nil
}

func (f *FederationEngineConfigV2Factory) engineConfigArguments(fieldConfs *plan.FieldConfigurations, generatedArgs map[TypeFieldLookupKey]TypeFieldArguments) {
	for i := range *fieldConfs {
		if len(generatedArgs) == 0 {
			return
		}

		lookupKey := CreateTypeFieldLookupKey((*fieldConfs)[i].TypeName, (*fieldConfs)[i].FieldName)
		currentArgs, ok := generatedArgs[lookupKey]
		if !ok {
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

func (f *FederationEngineConfigV2Factory) createArgumentConfigurationsForArgumentNames(argumentNames []string) plan.ArgumentsConfigurations {
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
