package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"io"
)

type ResolverArgs interface {
	ByKey(key []byte) []byte
	Dump() []string
	Keys() [][]byte
}

type DataSource interface {
	Resolve(ctx context.Context, args ResolverArgs, out io.Writer) (n int, err error)
}

type Planner interface {
	CorePlanner
	PlannerVisitors
}

type CorePlanner interface {
	// Plan plan returns the pre configured DataSource as well as the Arguments
	// During runtime the arguments get resolved and passed to the DataSource
	Plan(args []Argument) (DataSource, []Argument)
	// Configure is the function to initialize all important values for the Planner to function correctly
	// You probably need access to the Walker, Operation and Definition to use the Planner to its full power
	// Walker gives you useful information from within all visitor Callbacks, e.g. the Path & Ancestors
	// Operation is the AST of the GraphQL Operation
	// Definition is the AST of the GraphQL schema Definition
	// Args are the pre-calculated Arguments from the planner
	// resolverParameters are the parameters from the @directive params field
	Configure(operation, definition *ast.Document, walker *astvisitor.Walker)
}

type PlannerVisitors interface {
	astvisitor.EnterInlineFragmentVisitor
	astvisitor.LeaveInlineFragmentVisitor
	astvisitor.EnterSelectionSetVisitor
	astvisitor.LeaveSelectionSetVisitor
	astvisitor.EnterFieldVisitor
	astvisitor.LeaveFieldVisitor
}

type PlannerFactory interface {
	DataSourcePlanner() Planner
}

type PlannerFactoryFactory interface {
	Initialize(base BasePlanner, configReader io.Reader) (PlannerFactory, error)
}

type BasePlanner struct {
	Log                   abstractlogger.Logger
	Walker                *astvisitor.Walker   // nolint
	Definition, Operation *ast.Document        // nolint
	Args                  []Argument           // nolint
	RootField             rootField            // nolint
	Config                PlannerConfiguration // nolint
}

func NewBaseDataSourcePlanner(schema []byte, config PlannerConfiguration, logger abstractlogger.Logger) (*BasePlanner, error) {

	schema = append(schema, graphqlDefinitionBoilerplate...)

	definition, report := astparser.ParseGraphqlDocumentBytes(schema)
	if report.HasErrors() {
		return nil, report
	}

	return &BasePlanner{
		Config:     config,
		Log:        logger,
		Definition: &definition,
	}, nil
}

func (b *BasePlanner) Configure(operation, definition *ast.Document, walker *astvisitor.Walker) {
	b.Operation, b.Definition, b.Walker = operation, definition, walker
}

func (b *BasePlanner) RegisterDataSourcePlannerFactory(dataSourceName string, factory PlannerFactoryFactory) (err error) {
	for i := range b.Config.TypeFieldConfigurations {
		if dataSourceName != b.Config.TypeFieldConfigurations[i].DataSource.Name {
			continue
		}
		configReader := bytes.NewReader(b.Config.TypeFieldConfigurations[i].DataSource.Config)
		b.Config.TypeFieldConfigurations[i].DataSourcePlannerFactory, err = factory.Initialize(*b, configReader)
		if err != nil {
			return err
		}
	}
	return nil
}

type PlannerConfiguration struct {
	TypeFieldConfigurations []TypeFieldConfiguration
}

type TypeFieldConfiguration struct {
	TypeName                 string
	FieldName                string
	Mapping                  *MappingConfiguration
	DataSource               SourceConfig `json:"data_source"`
	DataSourcePlannerFactory PlannerFactory
}

type SourceConfig struct {
	// Kind defines the unique identifier of the DataSource
	// Kind needs to match to the Planner "DataSourceName" name
	Name string `json:"kind"`
	// Config is the DataSource specific configuration object
	// Each Planner needs to make sure to parse their Config Object correctly
	Config json.RawMessage `json:"dataSourceConfig"`
}

type MappingConfiguration struct {
	Disabled bool
	Path     string
}

func (p *PlannerConfiguration) DataSourcePlannerFactoryForTypeField(typeName, fieldName string) PlannerFactory {
	for i := range p.TypeFieldConfigurations {
		if p.TypeFieldConfigurations[i].TypeName == typeName && p.TypeFieldConfigurations[i].FieldName == fieldName {
			return p.TypeFieldConfigurations[i].DataSourcePlannerFactory
		}
	}
	return nil
}

func (p *PlannerConfiguration) MappingForTypeField(typeName, fieldName string) *MappingConfiguration {
	for i := range p.TypeFieldConfigurations {
		if p.TypeFieldConfigurations[i].TypeName == typeName && p.TypeFieldConfigurations[i].FieldName == fieldName {
			return p.TypeFieldConfigurations[i].Mapping
		}
	}
	return nil
}

type rootField struct {
	isDefined bool
	ref       int
}

func (r *rootField) SetIfNotDefined(ref int) {
	if r.isDefined {
		return
	}
	r.isDefined = true
	r.ref = ref
}

func (r *rootField) IsDefinedAndEquals(ref int) bool {
	return r.isDefined && r.ref == ref
}

type visitingDataSourcePlanner struct {
	CorePlanner
}

func (_ visitingDataSourcePlanner) EnterInlineFragment(ref int) {}
func (_ visitingDataSourcePlanner) LeaveInlineFragment(ref int) {}
func (_ visitingDataSourcePlanner) EnterSelectionSet(ref int)   {}
func (_ visitingDataSourcePlanner) LeaveSelectionSet(ref int)   {}
func (_ visitingDataSourcePlanner) EnterField(ref int)          {}
func (_ visitingDataSourcePlanner) LeaveField(ref int)          {}

func SimpleDataSourcePlanner(core CorePlanner) Planner {
	return &visitingDataSourcePlanner{
		CorePlanner: core,
	}
}

type Argument interface {
	ArgName() []byte
}

type ContextVariableArgument struct {
	Name         []byte
	VariableName []byte
}

func (c *ContextVariableArgument) ArgName() []byte {
	return c.Name
}

type PathSelector struct {
	Path string
}

type ObjectVariableArgument struct {
	Name         []byte
	PathSelector PathSelector
}

func (o *ObjectVariableArgument) ArgName() []byte {
	return o.Name
}

type StaticVariableArgument struct {
	Name  []byte
	Value []byte
}

func (s *StaticVariableArgument) ArgName() []byte {
	return s.Name
}

type ListArgument struct {
	Name      []byte
	Arguments []Argument
}

func (l ListArgument) ArgName() []byte {
	return l.Name
}

var graphqlDefinitionBoilerplate = []byte(`
"The 'Int' scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int
"The 'Float' scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float
"The 'String' scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String
"The 'Boolean' scalar type represents 'true' or 'false' ."
scalar Boolean
"The 'ID' scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as '4') or integer (such as 4) input value will be accepted as an ID."
scalar ID
"Directs the executor to include this field or fragment only when the argument is true."
directive @include(
    " Included when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Directs the executor to skip this field or fragment when the argument is true."
directive @skip(
    "Skipped when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Marks an element of a GraphQL schema as no longer supported."
directive @deprecated(
    """
    Explains why this element was deprecated, usually also including a suggestion
    for how to access supported similar data. Formatted in
    [Markdown](https://daringfireball.net/projects/markdown/).
    """
    reason: String = "No longer supported"
) on FIELD_DEFINITION | ENUM_VALUE

"""
A Directive provides a way to describe alternate runtime execution and type validation behavior in a GraphQL document.
In some cases, you need to provide options to alter GraphQL's execution behavior
in ways field arguments will not suffice, such as conditionally including or
skipping a field. Directives provide this by describing additional information
to the executor.
"""
type __Directive {
    name: String!
    description: String
    locations: [__DirectiveLocation!]!
    Args: [__InputValue!]!
}

"""
A Directive can be adjacent to many parts of the GraphQL language, a
__DirectiveLocation describes one such possible adjacencies.
"""
enum __DirectiveLocation {
    "Location adjacent to a query Operation."
    QUERY
    "Location adjacent to a mutation Operation."
    MUTATION
    "Location adjacent to a subscription Operation."
    SUBSCRIPTION
    "Location adjacent to a field."
    FIELD
    "Location adjacent to a fragment Definition."
    FRAGMENT_DEFINITION
    "Location adjacent to a fragment spread."
    FRAGMENT_SPREAD
    "Location adjacent to an inline fragment."
    INLINE_FRAGMENT
    "Location adjacent to a schema Definition."
    SCHEMA
    "Location adjacent to a scalar Definition."
    SCALAR
    "Location adjacent to an object type Definition."
    OBJECT
    "Location adjacent to a field Definition."
    FIELD_DEFINITION
    "Location adjacent to an argument Definition."
    ARGUMENT_DEFINITION
    "Location adjacent to an interface Definition."
    INTERFACE
    "Location adjacent to a union Definition."
    UNION
    "Location adjacent to an enum Definition."
    ENUM
    "Location adjacent to an enum value Definition."
    ENUM_VALUE
    "Location adjacent to an input object type Definition."
    INPUT_OBJECT
    "Location adjacent to an input object field Definition."
    INPUT_FIELD_DEFINITION
}
"""
One possible value for a given Enum. Enum values are unique values, not a
placeholder for a string or numeric value. However an Enum value is returned in
a JSON response as a string.
"""
type __EnumValue {
    name: String!
    description: String
    isDeprecated: Boolean!
    deprecationReason: String
}

"""
Object and Interface types are described by a list of Fields, each of which has
a name, potentially a list of arguments, and a return type.
"""
type __Field {
    name: String!
    description: String
    Args: [__InputValue!]!
    type: __Type!
    isDeprecated: Boolean!
    deprecationReason: String
}

"""Arguments provided to Fields or Directives and the input fields of an
InputObject are represented as Input Values which describe their type and
optionally a default value.
"""
type __InputValue {
    name: String!
    description: String
    type: __Type!
    "A GraphQL-formatted string representing the default value for this input value."
    defaultValue: String
}

"""
A GraphQL Schema defines the capabilities of a GraphQL server. It exposes all
available types and directives on the server, as well as the entry points for
query, mutation, and subscription operations.
"""
type __Schema {
    "A list of all types supported by this server."
    types: [__Type!]!
    "The type that query operations will be rooted at."
    queryType: __Type!
    "If this server supports mutation, the type that mutation operations will be rooted at."
    mutationType: __Type
    "If this server support subscription, the type that subscription operations will be rooted at."
    subscriptionType: __Type
    "A list of all directives supported by this server."
    directives: [__Directive!]!
}

"""
The fundamental unit of any GraphQL Schema is the type. There are many kinds of
types in GraphQL as represented by the '__TypeKind' enum.

Depending on the kind of a type, certain fields describe information about that
type. Scalar types provide no information beyond a name and description, while
Enum types provide their values. Object and Interface types provide the fields
they describe. Abstract types, Union and Interface, provide the Object types
possible at runtime. List and NonNull types compose other types.
"""
type __Type {
    kind: __TypeKind!
    name: String
    description: String
    fields(includeDeprecated: Boolean = false): [__Field!]
    interfaces: [__Type!]
    possibleTypes: [__Type!]
    enumValues(includeDeprecated: Boolean = false): [__EnumValue!]
    inputFields: [__InputValue!]
    ofType: __Type
}

"An enum describing what kind of type a given '__Type' is."
enum __TypeKind {
    "Indicates this type is a scalar."
    SCALAR
    "Indicates this type is an object. 'fields' and 'interfaces' are valid fields."
    OBJECT
    "Indicates this type is an interface. 'fields' ' and ' 'possibleTypes' are valid fields."
    INTERFACE
    "Indicates this type is a union. 'possibleTypes' is a valid field."
    UNION
    "Indicates this type is an enum. 'enumValues' is a valid field."
    ENUM
    "Indicates this type is an input object. 'inputFields' is a valid field."
    INPUT_OBJECT
    "Indicates this type is a list. 'ofType' is a valid field."
    LIST
    "Indicates this type is a non-null. 'ofType' is a valid field."
    NON_NULL
}`)
