package plan

import (
	"context"
	"encoding/json"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type PlannerFactory interface {
	// Planner should return the DataSourcePlanner
	// closer is the closing channel for all stateful DataSources
	// At runtime, the Execution Engine will be instantiated with one global resolve.Closer.
	// Once the Closer gets closed, all stateful DataSources must close their connections and cleanup themselves.
	// They can do so by starting a goroutine on instantiation time that blocking reads on the resolve.Closer.
	// Once the Closer emits the close event, they have to terminate (e.g. close database connections).
	Planner(ctx context.Context) DataSourcePlanner
}

type TypeField struct {
	TypeName   string
	FieldNames []string
}

type AlternativeTypeField struct {
	TypeField
	AncestorNode TypeField
}

type DataSourceConfiguration struct {
	// RootNodes - defines the nodes where the responsibility of the DataSource begins
	// When you enter a node, and it is not a child node
	// when you have entered into a field which representing data source - it means that we are starting a new planning stage
	RootNodes []TypeField
	// ChildNodes - describes additional fields which will be requested along with fields which has a datasources
	// They are always required for the Graphql datasources cause each field could have its own datasource
	// For any single point datasource like HTTP/REST or GRPC we could not request fewer fields, as we always get a full response
	ChildNodes       []TypeField
	AlternativeNodes []AlternativeTypeField
	Directives       DirectiveConfigurations
	Factory          PlannerFactory
	Custom           json.RawMessage

	TypeConfigurations                   TypeConfigurations
	FieldConfigurations                  FieldConfigurations
	FieldConfigurationsFromParentPlanner FieldConfigurations
}

func (d *DataSourceConfiguration) HasRootNode(typeName, fieldName string) bool {
	for i := range d.RootNodes {
		if typeName != d.RootNodes[i].TypeName {
			continue
		}
		for j := range d.RootNodes[i].FieldNames {
			if fieldName == d.RootNodes[i].FieldNames[j] {
				return true
			}
		}
	}
	return false
}

func (d *DataSourceConfiguration) HasRootNodeWithTypename(typeName string) bool {
	for i := range d.RootNodes {
		if typeName != d.RootNodes[i].TypeName {
			continue
		}
		return true
	}
	return false
}

func (d *DataSourceConfiguration) HasChildNode(typeName, fieldName string) bool {
	for i := range d.ChildNodes {
		if typeName != d.ChildNodes[i].TypeName {
			continue
		}
		for j := range d.ChildNodes[i].FieldNames {
			if fieldName == d.ChildNodes[i].FieldNames[j] {
				return true
			}
		}
	}
	return false
}

func (d *DataSourceConfiguration) HasChildNodeWithTypename(typeName string) bool {
	for i := range d.ChildNodes {
		if typeName != d.ChildNodes[i].TypeName {
			continue
		}
		return true
	}
	return false
}

func (d *DataSourceConfiguration) HasFieldConfiguration(typeName, requiresFields string) bool {
	for i := range d.FieldConfigurations {
		if typeName != d.FieldConfigurations[i].TypeName {
			continue
		}
		if d.FieldConfigurations[i].RequiresFieldsSelectionSet == requiresFields {
			return true
		}
	}
	return false
}

func (d *DataSourceConfiguration) FieldConfigurationsForType(typeName string) []FieldConfiguration {
	return d.FieldConfigurations.FilterByType(typeName)
}

func (d *DataSourceConfiguration) FieldConfigurationsForTypeAndField(typeName, fieldName string) []FieldConfiguration {
	return d.FieldConfigurations.FilterByTypeAndField(typeName, fieldName)
}

type DirectiveConfigurations []DirectiveConfiguration

type DirectiveConfiguration struct {
	DirectiveName string
	RenameTo      string
}

func (d *DirectiveConfigurations) RenameTypeNameOnMatchStr(directiveName string) string {
	for i := range *d {
		if (*d)[i].DirectiveName == directiveName {
			return (*d)[i].RenameTo
		}
	}
	return directiveName
}

func (d *DirectiveConfigurations) RenameTypeNameOnMatchBytes(directiveName []byte) []byte {
	str := string(directiveName)
	for i := range *d {
		if (*d)[i].DirectiveName == str {
			return []byte((*d)[i].RenameTo)
		}
	}
	return directiveName
}

type DataSourcePlanningBehavior struct {
	// MergeAliasedRootNodes will reuse a data source for multiple root fields with aliases if true.
	// Example:
	//  {
	//    rootField
	//    alias: rootField
	//  }
	// On dynamic data sources (e.g. GraphQL, SQL, ...) this should return true and for
	// static data sources (e.g. REST, static, GRPC...) it should be false.
	MergeAliasedRootNodes bool
	// OverrideFieldPathFromAlias will let the planner know if the response path should also be aliased (= true)
	// or not (= false)
	// Example:
	//  {
	//    rootField
	//    alias: original
	//  }
	// When true expected response will be { "rootField": ..., "alias": ... }
	// When false expected response will be { "rootField": ..., "original": ... }
	OverrideFieldPathFromAlias bool
	// IncludeTypeNameFields should be set to true if the planner wants to get EnterField & LeaveField events
	// for __typename fields
	IncludeTypeNameFields bool
}

type DataSourcePlanner interface {
	Register(visitor *Visitor, configuration DataSourceConfiguration, isNested bool) error
	ConfigureFetch() FetchConfiguration
	ConfigureSubscription() SubscriptionConfiguration
	DataSourcePlanningBehavior() DataSourcePlanningBehavior
	// DownstreamResponseFieldAlias allows the DataSourcePlanner to overwrite the response path with an alias
	// It's required to set OverrideFieldPathFromAlias to true
	// This function is useful in the following scenario
	// 1. The downstream Query doesn't contain an alias
	// 2. The path configuration rewrites the field to an existing field
	// 3. The DataSourcePlanner is using an alias to the upstream
	// Example:
	//
	// type Query {
	//		country: Country
	//		countryAlias: Country
	// }
	//
	// Both, country and countryAlias have a path in the FieldConfiguration of "country"
	// In theory, they would be treated as the same field
	// However, by using DownstreamResponseFieldAlias, it's possible for the DataSourcePlanner to use an alias for countryAlias.
	// In this case, the response would contain both, country and countryAlias fields in the response.
	// At the same time, the downstream Query would only expect the response on the path "country",
	// as both country and countryAlias have a mapping to the path "country".
	// The DataSourcePlanner could keep track that it rewrites the upstream query and use DownstreamResponseFieldAlias
	// to indicate to the Planner to expect the response for countryAlias on the path "countryAlias" instead of "country".
	DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool)
}

type SubscriptionConfiguration struct {
	Input                 string
	Variables             resolve.Variables
	DataSource            resolve.SubscriptionDataSource
	ProcessResponseConfig resolve.ProcessResponseConfig
}

type FetchConfiguration struct {
	Input                string
	Variables            resolve.Variables
	DataSource           resolve.DataSource
	DisallowSingleFlight bool
	// DisableDataLoader will configure the Resolver to not use DataLoader
	// If this is set to false, the planner might still decide to override it,
	// e.g. if a field depends on an exported variable which doesn't work with DataLoader
	DisableDataLoader     bool
	DisallowParallelFetch bool
	ProcessResponseConfig resolve.ProcessResponseConfig
	BatchConfig           BatchConfig
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
}

type BatchConfig struct {
	AllowBatch   bool
	BatchFactory resolve.DataSourceBatchFactory
}
