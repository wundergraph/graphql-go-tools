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

// PlannerFactory creates concrete DataSourcePlanner's.
// For stateful datasources, the factory should contain execution context
// Once the context gets canceled, all stateful DataSources must close their connections and cleanup themselves.
type PlannerFactory[DataSourceSpecificConfiguration any] interface {

	// Planner creates a new DataSourcePlanner
	Planner(logger abstractlogger.Logger) DataSourcePlanner[DataSourceSpecificConfiguration]

	// Context returns the execution context of the factory
	// For stateful datasources, the factory should contain cancellable global execution context
	// This method serves as a flag that factory should have a context
	Context() context.Context

	UpstreamSchema(dataSourceConfig DataSourceConfiguration[DataSourceSpecificConfiguration]) (*ast.Document, bool)
	PlanningBehavior() DataSourcePlanningBehavior
}

type DataSourceMetadata struct {
	// FederationMetaData has federation-specific configuration for entity interfaces and
	// the @key, @requires, @provides directives.
	FederationMetaData

	// RootNodes defines the nodes where the responsibility of the DataSource begins.
	// RootNode is a node from which we could start a query or a subquery.
	// For a federation, RootNodes contain root query type fields, entity type fields,
	// and entity object fields.
	RootNodes TypeFields

	// ChildNodes describes additional fields, which are requested along with fields that the datasource has.
	// They're always required for Graphql datasources because each field could have its own datasource.
	// For a flat datasource (HTTP/REST or GRPC) we cannot request fewer fields, as we always get a full response.
	// For a federation, ChildNodes contain non-entity type fields and interface type fields.
	// Unions shouldn't be present in the child or root nodes.
	ChildNodes TypeFields

	Directives *DirectiveConfigurations

	rootNodesIndex  map[string]fieldsIndex // maps TypeName to fieldsIndex
	childNodesIndex map[string]fieldsIndex // maps TypeName to fieldsIndex

	// requireFetchReasons provides a lookup map for fields marked with corresponding directive.
	requireFetchReasons map[FieldCoordinate]struct{}
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
	RequireFetchReasons() map[FieldCoordinate]struct{}
}

type fieldsIndex struct {
	fields         map[string]struct{}
	externalFields map[string]struct{}
}

func (d *DataSourceMetadata) Init() error {
	d.InitNodesIndex()
	return d.InitKeys()
}

func (d *DataSourceMetadata) InitKeys() error {
	for i := 0; i < len(d.FederationMetaData.Keys); i++ {
		if err := d.FederationMetaData.Keys[i].parseSelectionSet(); err != nil {
			return err
		}
	}

	return nil
}

func (d *DataSourceMetadata) InitNodesIndex() {
	if d == nil {
		return
	}

	d.rootNodesIndex = make(map[string]fieldsIndex, len(d.RootNodes))
	d.childNodesIndex = make(map[string]fieldsIndex, len(d.ChildNodes))
	d.requireFetchReasons = make(map[FieldCoordinate]struct{})

	for i := range d.RootNodes {
		typeName := d.RootNodes[i].TypeName
		if _, ok := d.rootNodesIndex[typeName]; !ok {
			d.rootNodesIndex[typeName] = fieldsIndex{
				fields:         make(map[string]struct{}, len(d.RootNodes[i].FieldNames)),
				externalFields: make(map[string]struct{}, len(d.RootNodes[i].ExternalFieldNames)),
			}
		}
		for _, name := range d.RootNodes[i].FieldNames {
			d.rootNodesIndex[typeName].fields[name] = struct{}{}
		}
		for _, name := range d.RootNodes[i].ExternalFieldNames {
			d.rootNodesIndex[typeName].externalFields[name] = struct{}{}
		}
		for _, name := range d.RootNodes[i].FetchReasonFields {
			d.requireFetchReasons[FieldCoordinate{typeName, name}] = struct{}{}
		}
	}

	for i := range d.ChildNodes {
		typeName := d.ChildNodes[i].TypeName
		if _, ok := d.childNodesIndex[typeName]; !ok {
			d.childNodesIndex[typeName] = fieldsIndex{
				fields:         make(map[string]struct{}),
				externalFields: make(map[string]struct{}),
			}
		}
		for _, name := range d.ChildNodes[i].FieldNames {
			d.childNodesIndex[typeName].fields[name] = struct{}{}
		}
		for _, name := range d.ChildNodes[i].ExternalFieldNames {
			d.childNodesIndex[typeName].externalFields[name] = struct{}{}
		}
		for _, name := range d.ChildNodes[i].FetchReasonFields {
			d.requireFetchReasons[FieldCoordinate{typeName, name}] = struct{}{}
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

func (d *DataSourceMetadata) RequireFetchReasons() map[FieldCoordinate]struct{} {
	return d.requireFetchReasons
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

	if metadata != nil {
		if err := metadata.Init(); err != nil {
			return nil, err
		}
	}

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

type DataSource interface {
	FederationInfo
	NodesInfo
	DirectivesConfigurations

	UpstreamSchema() (*ast.Document, bool)
	PlanningBehavior() DataSourcePlanningBehavior

	Id() string
	Name() string
	Hash() DSHash
	FederationConfiguration() FederationMetaData
	CreatePlannerConfiguration(logger abstractlogger.Logger, fetchConfig *objectFetchConfiguration, pathConfig *plannerPathsConfiguration, configuration *Configuration) PlannerConfiguration
}

func (d *dataSourceConfiguration[T]) CustomConfiguration() T {
	return d.custom
}

func (d *dataSourceConfiguration[T]) CreatePlannerConfiguration(logger abstractlogger.Logger, fetchConfig *objectFetchConfiguration, pathConfig *plannerPathsConfiguration, configuration *Configuration) PlannerConfiguration {
	planner := d.factory.Planner(logger)

	fetchConfig.planner = planner

	plannerConfig := &plannerConfiguration[T]{
		dataSourceConfiguration:   d,
		objectFetchConfiguration:  fetchConfig,
		plannerPathsConfiguration: pathConfig,
		planner:                   planner,
		options: plannerConfigurationOptions{
			EnableOperationNamePropagation: configuration.EnableOperationNamePropagation,
		},
	}

	return plannerConfig
}

func (d *dataSourceConfiguration[T]) UpstreamSchema() (*ast.Document, bool) {
	return d.factory.UpstreamSchema(d)
}

func (d *dataSourceConfiguration[T]) PlanningBehavior() DataSourcePlanningBehavior {
	return d.factory.PlanningBehavior()
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
	Options        plannerConfigurationOptions
	FetchID        int
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

// DataSourcePlanningBehavior contains DataSource-specific planning flags.
type DataSourcePlanningBehavior struct {
	// MergeAliasedRootNodes set to true will reuse a data source for multiple root fields with aliases.
	// Example:
	//  {
	//    rootField
	//    alias: rootField
	//  }
	// On dynamic data sources (GraphQL, SQL) this should be set to true,
	// and for static data sources (REST, static, gRPC) it should be false.
	MergeAliasedRootNodes bool

	// OverrideFieldPathFromAlias set to true will let the planner know
	// if the response path should also be aliased.
	//
	// Example:
	//  {
	//    rootField
	//    alias: original
	//  }
	// When true expected response will be { "rootField": ..., "alias": ... }
	// When false expected response will be { "rootField": ..., "original": ... }
	OverrideFieldPathFromAlias bool

	// AllowPlanningTypeName set to true will allow the planner to plan __typename fields.
	AllowPlanningTypeName bool

	// If true then planner will rewrite the operation
	// to flatten inline fragments to only the concrete types.
	AlwaysFlattenFragments bool
}

type DataSourceFetchPlanner interface {
	ConfigureFetch() resolve.FetchConfiguration
	ConfigureSubscription() SubscriptionConfiguration
}

type DataSourceBehavior interface {
	// DownstreamResponseFieldAlias allows the DataSourcePlanner to overwrite the response path with an alias.
	// It requires DataSourcePlanningBehavior.OverrideFieldPathFromAlias to be set to true.
	// This function is useful in the following scenarios:
	// 1. The downstream Query doesn't contain an alias,
	// 2. The path configuration rewrites the field to an existing field,
	// 3. The DataSourcePlanner using an alias to the upstream.
	//
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
