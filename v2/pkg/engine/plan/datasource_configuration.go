package plan

import (
	"context"
	"errors"

	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type DSHash uint64

// PlannerFactory is the factory for the creation of the concrete DataSourcePlanner
// For stateful datasources, the factory should contain execution context
// Once the context gets cancelled, all stateful DataSources must close their connections and cleanup themselves.
type PlannerFactory[DataSourceSpecificConfiguration any] interface {
	// Planner creates a new DataSourcePlanner
	Planner(logger abstractlogger.Logger) DataSourcePlanner[DataSourceSpecificConfiguration]
	// Context returns the execution context of the factory
	// For stateful datasources, the factory should contain cancellable global execution context
	// This method serves as a flag that factory should have a context
	Context() context.Context
	UpstreamSchema(dataSourceConfig DataSourceConfiguration[DataSourceSpecificConfiguration]) (*ast.Document, bool)
}

type DataSourceMetadata struct {
	// FederationMetaData - describes the behavior of the DataSource in the context of the Federation
	FederationMetaData

	// RootNodes - defines the nodes where the responsibility of the DataSource begins
	// RootNode is a node from which you could start a query or a subquery
	// Note: for federation root nodes are root query type fields, entity type fields, and entity object fields
	RootNodes TypeFields
	// ChildNodes - describes additional fields which will be requested along with fields which has a datasources
	// They are always required for the Graphql datasources cause each field could have its own datasource
	// For any flat datasource like HTTP/REST or GRPC we could not request fewer fields, as we always get a full response
	// Note: for federation child nodes are non-entity type fields and interface type fields
	// Note: Unions are not present in the child or root nodes
	ChildNodes TypeFields
	Directives *DirectiveConfigurations

	rootNodesIndex  map[string]fieldsIndex
	childNodesIndex map[string]fieldsIndex
}

type DirectivesConfigurations interface {
	DirectiveConfigurations() *DirectiveConfigurations
}

type NodesAccess interface {
	ListRootNodes() TypeFields
	ListChildNodes() TypeFields
}

type NodesInfo interface {
	HasRootNode(typeName, fieldName string) bool
	HasExternalRootNode(typeName, fieldName string) bool
	HasRootNodeWithTypename(typeName string) bool
	HasChildNode(typeName, fieldName string) bool
	HasExternalChildNode(typeName, fieldName string) bool
	HasChildNodeWithTypename(typeName string) bool
}

type fieldsIndex struct {
	fields         map[string]struct{}
	externalFields map[string]struct{}
}

func (d *DataSourceMetadata) InitNodesIndex() {
	if d == nil {
		return
	}

	d.rootNodesIndex = make(map[string]fieldsIndex, len(d.RootNodes))
	d.childNodesIndex = make(map[string]fieldsIndex, len(d.ChildNodes))

	for i := range d.RootNodes {
		if _, ok := d.rootNodesIndex[d.RootNodes[i].TypeName]; !ok {
			d.rootNodesIndex[d.RootNodes[i].TypeName] = fieldsIndex{
				make(map[string]struct{}, len(d.RootNodes[i].FieldNames)),
				make(map[string]struct{}, len(d.RootNodes[i].ExternalFieldNames)),
			}
		}
		for j := range d.RootNodes[i].FieldNames {
			d.rootNodesIndex[d.RootNodes[i].TypeName].fields[d.RootNodes[i].FieldNames[j]] = struct{}{}
		}
		for j := range d.RootNodes[i].ExternalFieldNames {
			d.rootNodesIndex[d.RootNodes[i].TypeName].externalFields[d.RootNodes[i].ExternalFieldNames[j]] = struct{}{}
		}
	}

	for i := range d.ChildNodes {
		if _, ok := d.childNodesIndex[d.ChildNodes[i].TypeName]; !ok {
			d.childNodesIndex[d.ChildNodes[i].TypeName] = fieldsIndex{
				make(map[string]struct{}),
				make(map[string]struct{}),
			}
		}
		for j := range d.ChildNodes[i].FieldNames {
			d.childNodesIndex[d.ChildNodes[i].TypeName].fields[d.ChildNodes[i].FieldNames[j]] = struct{}{}
		}
		for j := range d.ChildNodes[i].ExternalFieldNames {
			d.childNodesIndex[d.ChildNodes[i].TypeName].externalFields[d.ChildNodes[i].ExternalFieldNames[j]] = struct{}{}
		}
	}
}

func (d *DataSourceMetadata) DirectiveConfigurations() *DirectiveConfigurations {
	return d.Directives
}

func (d *DataSourceMetadata) HasRootNode(typeName, fieldName string) bool {
	if d.rootNodesIndex == nil {
		return false
	}

	index, ok := d.rootNodesIndex[typeName]
	if !ok {
		return false
	}

	_, ok = index.fields[fieldName]
	return ok
}

func (d *DataSourceMetadata) HasExternalRootNode(typeName, fieldName string) bool {
	if d.rootNodesIndex == nil {
		return false
	}
	index, ok := d.rootNodesIndex[typeName]
	if !ok {
		return false
	}
	_, ok = index.externalFields[fieldName]
	return ok
}

func (d *DataSourceMetadata) HasRootNodeWithTypename(typeName string) bool {
	if d.rootNodesIndex == nil {
		return false
	}
	_, ok := d.rootNodesIndex[typeName]
	return ok
}

func (d *DataSourceMetadata) HasChildNode(typeName, fieldName string) bool {
	if d.childNodesIndex == nil {
		return false
	}
	index, ok := d.childNodesIndex[typeName]
	if !ok {
		return false
	}

	_, ok = index.fields[fieldName]
	return ok
}

func (d *DataSourceMetadata) HasExternalChildNode(typeName, fieldName string) bool {
	if d.childNodesIndex == nil {
		return false
	}
	index, ok := d.childNodesIndex[typeName]
	if !ok {
		return false
	}
	_, ok = index.externalFields[fieldName]
	return ok
}

func (d *DataSourceMetadata) HasChildNodeWithTypename(typeName string) bool {
	if d.childNodesIndex == nil {
		return false
	}
	_, ok := d.childNodesIndex[typeName]
	return ok
}

func (d *DataSourceMetadata) ListRootNodes() TypeFields {
	return d.RootNodes
}

func (d *DataSourceMetadata) ListChildNodes() TypeFields {
	return d.ChildNodes

}

// dataSourceConfiguration is the configuration for a DataSource
type dataSourceConfiguration[T any] struct {
	*DataSourceMetadata                   // DataSourceMetadata is the information about root and child nodes and federation metadata if applicable
	id                  string            // id is a unique identifier for the DataSource
	name                string            // name is a human-readable name for the DataSource
	factory             PlannerFactory[T] // factory is the factory for the creation of the concrete DataSourcePlanner
	custom              T                 // custom is the datasource specific configuration

	hash DSHash // hash is a unique hash for the dataSourceConfiguration used to match datasources
}

func NewDataSourceConfiguration[T any](id string, factory PlannerFactory[T], metadata *DataSourceMetadata, customConfig T) (DataSourceConfiguration[T], error) {
	return NewDataSourceConfigurationWithName(id, id, factory, metadata, customConfig)
}

func NewDataSourceConfigurationWithName[T any](id string, name string, factory PlannerFactory[T], metadata *DataSourceMetadata, customConfig T) (DataSourceConfiguration[T], error) {
	if id == "" {
		return nil, errors.New("data source id could not be empty")
	}

	metadata.InitNodesIndex()

	return &dataSourceConfiguration[T]{
		DataSourceMetadata: metadata,
		id:                 id,
		name:               name,
		factory:            factory,
		custom:             customConfig,
		hash:               DSHash(xxhash.Sum64([]byte(id))),
	}, nil
}

type DataSourceConfiguration[T any] interface {
	DataSource
	CustomConfiguration() T
}

type DataSourceUpstreamSchema interface {
	UpstreamSchema() (*ast.Document, bool)
}

type DataSource interface {
	FederationInfo
	NodesInfo
	DirectivesConfigurations
	DataSourceUpstreamSchema
	Id() string
	Name() string
	Hash() DSHash
	FederationConfiguration() FederationMetaData
	CreatePlannerConfiguration(logger abstractlogger.Logger, fetchConfig *objectFetchConfiguration, pathConfig *plannerPathsConfiguration) PlannerConfiguration
}

func (d *dataSourceConfiguration[T]) CustomConfiguration() T {
	return d.custom
}

func (d *dataSourceConfiguration[T]) CreatePlannerConfiguration(logger abstractlogger.Logger, fetchConfig *objectFetchConfiguration, pathConfig *plannerPathsConfiguration) PlannerConfiguration {
	planner := d.factory.Planner(logger)

	fetchConfig.planner = planner

	plannerConfig := &plannerConfiguration[T]{
		dataSourceConfiguration:   d,
		objectFetchConfiguration:  fetchConfig,
		plannerPathsConfiguration: pathConfig,
		planner:                   planner,
	}

	return plannerConfig
}

func (d *dataSourceConfiguration[T]) UpstreamSchema() (*ast.Document, bool) {
	return d.factory.UpstreamSchema(d)
}

func (d *dataSourceConfiguration[T]) Id() string {
	return d.id
}

func (d *dataSourceConfiguration[T]) Name() string {
	return d.name
}

func (d *dataSourceConfiguration[T]) FederationConfiguration() FederationMetaData {
	return d.FederationMetaData
}

func (d *dataSourceConfiguration[T]) Hash() DSHash {
	return d.hash
}

type DataSourcePlannerConfiguration struct {
	RequiredFields FederationFieldConfigurations
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

type DirectiveConfigurations []DirectiveConfiguration

func NewDirectiveConfigurations(configs []DirectiveConfiguration) *DirectiveConfigurations {
	directiveConfigs := DirectiveConfigurations(configs)
	return &directiveConfigs
}

type DirectiveConfiguration struct {
	DirectiveName string
	RenameTo      string
}

func (d *DirectiveConfigurations) RenameTypeNameOnMatchStr(directiveName string) string {
	if d == nil {
		return directiveName
	}

	for i := range *d {
		if (*d)[i].DirectiveName == directiveName {
			return (*d)[i].RenameTo
		}
	}
	return directiveName
}

func (d *DirectiveConfigurations) RenameTypeNameOnMatchBytes(directiveName []byte) []byte {
	if d == nil {
		return directiveName
	}

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
	// IncludeTypeNameFields should be set to true if the planner allows to plan __typename fields
	IncludeTypeNameFields bool
}

type DataSourceFetchPlanner interface {
	ConfigureFetch() resolve.FetchConfiguration
	ConfigureSubscription() SubscriptionConfiguration
}

type DataSourceBehavior interface {
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

type Identifyable interface {
	astvisitor.VisitorIdentifier
}

type DataSourcePlanner[T any] interface {
	DataSourceFetchPlanner
	DataSourceBehavior
	Identifyable
	Register(visitor *Visitor, configuration DataSourceConfiguration[T], dataSourcePlannerConfiguration DataSourcePlannerConfiguration) error
}

type SubscriptionConfiguration struct {
	Input          string
	Variables      resolve.Variables
	DataSource     resolve.SubscriptionDataSource
	PostProcessing resolve.PostProcessingConfiguration
	QueryPlan      *resolve.QueryPlan
}
