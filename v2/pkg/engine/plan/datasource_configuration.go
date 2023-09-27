package plan

import (
	"context"
	"encoding/json"

	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type DSHash uint64

type PlannerFactory interface {
	// Planner should return the DataSourcePlanner
	// closer is the closing channel for all stateful DataSources
	// At runtime, the Execution Engine will be instantiated with one global resolve.Closer.
	// Once the Closer gets closed, all stateful DataSources must close their connections and cleanup themselves.
	// They can do so by starting a goroutine on instantiation time that blocking reads on the resolve.Closer.
	// Once the Closer emits the close event, they have to terminate (e.g. close database connections).
	Planner(ctx context.Context) DataSourcePlanner
}

type DataSourceConfiguration struct {
	// RootNodes - defines the nodes where the responsibility of the DataSource begins
	// When you enter a node, and it is not a child node
	// when you have entered into a field which representing data source - it means that we are starting a new planning stage
	RootNodes TypeFields
	// ChildNodes - describes additional fields which will be requested along with fields which has a datasources
	// They are always required for the Graphql datasources cause each field could have its own datasource
	// For any single point datasource like HTTP/REST or GRPC we could not request fewer fields, as we always get a full response
	ChildNodes TypeFields
	Directives DirectiveConfigurations
	Factory    PlannerFactory
	Custom     json.RawMessage

	FederationMetaData FederationMetaData

	hash DSHash
}

func (d *DataSourceConfiguration) Hash() DSHash {
	if d.hash != 0 {
		return d.hash
	}
	d.hash = DSHash(xxhash.Sum64(d.Custom))
	return d.hash
}

type DataSourcePlannerConfiguration struct {
	RequiredFields FederationFieldConfigurations
	ProvidedFields NodeSuggestions
	ParentPath     string
	PathType       PlannerPathType
	IsNested       bool
}

type PlannerPathType int

const (
	PlannerPathObject PlannerPathType = iota
	PlannerPathArrayItem
	PlannerPathNestedInArray
)

func (c *DataSourcePlannerConfiguration) HasRequiredFields() bool {
	return len(c.RequiredFields) > 0
}

func (d *DataSourceConfiguration) HasRootNode(typeName, fieldName string) bool {
	return d.RootNodes.HasNode(typeName, fieldName)
}

func (d *DataSourceConfiguration) HasRootNodeWithTypename(typeName string) bool {
	return d.RootNodes.HasNodeWithTypename(typeName)
}

func (d *DataSourceConfiguration) HasChildNode(typeName, fieldName string) bool {
	return d.ChildNodes.HasNode(typeName, fieldName)
}

func (d *DataSourceConfiguration) HasChildNodeWithTypename(typeName string) bool {
	return d.ChildNodes.HasNodeWithTypename(typeName)
}

func (d *DataSourceConfiguration) HasKeyRequirement(typeName, requiresFields string) bool {
	return d.FederationMetaData.Keys.HasSelectionSet(typeName, "", requiresFields)
}

func (d *DataSourceConfiguration) RequiredFieldsByKey(typeName string) []FederationFieldConfiguration {
	return d.FederationMetaData.Keys.FilterByType(typeName)
}

func (d *DataSourceConfiguration) RequiredFieldsByRequires(typeName, fieldName string) []FederationFieldConfiguration {
	return d.FederationMetaData.Requires.FilterByTypeAndField(typeName, fieldName)
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
	Register(visitor *Visitor, configuration DataSourceConfiguration, dataSourcePlannerConfiguration DataSourcePlannerConfiguration) error
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
	Input          string
	Variables      resolve.Variables
	DataSource     resolve.SubscriptionDataSource
	PostProcessing resolve.PostProcessingConfiguration
}

type FetchConfiguration struct {
	Input                         string
	Variables                     resolve.Variables
	DataSource                    resolve.DataSource
	DisallowSingleFlight          bool
	RequiresSerialFetch           bool
	RequiresBatchFetch            bool
	RequiresParallelListItemFetch bool
	PostProcessing                resolve.PostProcessingConfiguration
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
}
