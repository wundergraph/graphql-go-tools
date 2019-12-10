//go:generate packr
package execution

import (
	"bytes"
	"encoding/json"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"
	"github.com/gobuffalo/packr"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"go.uber.org/zap"
	"io"
	"sort"
)

type Handler struct {
	log                *zap.Logger
	definition         ast.Document
	graphqlDefinitions *packr.Box
}

func NewHandler(schema []byte, logger *zap.Logger) (*Handler, error) {

	schema = append(schema, graphqlDefinitionBoilerplate...)

	definition, report := astparser.ParseGraphqlDocumentBytes(schema)
	if report.HasErrors() {
		return nil, report
	}

	box := packr.NewBox("./graphql_definitions")

	return &Handler{
		log:                logger,
		definition:         definition,
		graphqlDefinitions: &box,
	}, nil
}

func (h *Handler) RenderGraphQLDefinitions(out io.Writer) error {
	buf := bytes.Buffer{}
	files := h.graphqlDefinitions.List()
	sort.Strings(files)
	for i := 0; i < len(files); i++ {
		data, _ := h.graphqlDefinitions.Find(files[i])
		if len(data) == 0 {
			continue
		}
		_, err := buf.Write(append(data, literal.LINETERMINATOR...))
		if err != nil {
			return err
		}
	}
	doc, report := astparser.ParseGraphqlDocumentBytes(buf.Bytes())
	if report.HasErrors() {
		return report
	}
	return astprinter.PrintIndent(&doc, nil, []byte("  "), out)
}

type GraphqlRequest struct {
	OperationName string          `json:"operation_name"`
	Variables     json.RawMessage `json:"variables"`
	Query         string          `json:"query"`
}

func (h *Handler) Handle(requestData, extraVariables []byte) (executor *Executor, node RootNode, ctx Context, err error) {

	var graphqlRequest GraphqlRequest
	err = json.Unmarshal(requestData, &graphqlRequest)
	if err != nil {
		return
	}

	operationDocument, report := astparser.ParseGraphqlDocumentString(graphqlRequest.Query)
	if report.HasErrors() {
		err = report
		return
	}

	variables, extraArguments := h.VariablesFromJson(graphqlRequest.Variables, extraVariables)

	planner := NewPlanner(h.resolverDefinitions(&report))
	if report.HasErrors() {
		err = report
		return
	}

	astnormalization.NormalizeOperation(&operationDocument, &h.definition, &report)
	if report.HasErrors() {
		err = report
		return
	}

	validator := astvalidation.DefaultOperationValidator()
	if report.HasErrors() {
		err = report
		return
	}
	validator.Validate(&operationDocument, &h.definition, &report)
	if report.HasErrors() {
		err = report
		return
	}
	normalizer := astnormalization.NewNormalizer(true)
	normalizer.NormalizeOperation(&operationDocument, &h.definition, &report)
	if report.HasErrors() {
		err = report
		return
	}
	plan := planner.Plan(&operationDocument, &h.definition, &report)
	if report.HasErrors() {
		err = report
		return
	}

	executor = NewExecutor()
	ctx = Context{
		Variables:      variables,
		ExtraArguments: extraArguments,
	}

	return executor, plan, ctx, err
}

func (h *Handler) VariablesFromJson(requestVariables, extraVariables []byte) (variables Variables, extraArguments []Argument) {
	variables = map[uint64][]byte{}
	_ = jsonparser.ObjectEach(requestVariables, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		variables[xxhash.Sum64(key)] = value
		return nil
	})
	_ = jsonparser.ObjectEach(extraVariables, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		variables[xxhash.Sum64(key)] = value
		extraArguments = append(extraArguments, &ContextVariableArgument{
			Name:         key,
			VariableName: key,
		})
		return nil
	})
	return
}

func (h *Handler) resolverDefinitions(report *operationreport.Report) ResolverDefinitions {

	baseDataSourcePlanner := BaseDataSourcePlanner{
		log:                h.log,
		graphqlDefinitions: h.graphqlDefinitions,
	}

	definitions := ResolverDefinitions{
		{
			TypeName:  literal.QUERY,
			FieldName: literal.UNDERSCORESCHEMA,
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return NewSchemaDataSourcePlanner(&h.definition, report, baseDataSourcePlanner)
			},
		},
	}

	walker := astvisitor.NewWalker(8)
	visitor := resolverDefinitionsVisitor{
		Walker:     &walker,
		definition: &h.definition,
		resolvers:  &definitions,
		dataSourcePlannerFactories: []func() DataSourcePlanner{
			func() DataSourcePlanner {
				return NewGraphQLDataSourcePlanner(baseDataSourcePlanner)
			},
			func() DataSourcePlanner {
				return NewHttpJsonDataSourcePlanner(baseDataSourcePlanner)
			},
			func() DataSourcePlanner {
				return NewHttpPollingStreamDataSourcePlanner(baseDataSourcePlanner)
			},
			func() DataSourcePlanner {
				return NewStaticDataSourcePlanner(baseDataSourcePlanner)
			},
			func() DataSourcePlanner {
				return NewTypeDataSourcePlanner(baseDataSourcePlanner)
			},
			func() DataSourcePlanner {
				return NewNatsDataSourcePlanner(baseDataSourcePlanner)
			},
			func() DataSourcePlanner {
				return NewMQTTDataSourcePlanner(baseDataSourcePlanner)
			},
		},
	}
	walker.RegisterEnterFieldDefinitionVisitor(&visitor)
	walker.Walk(&h.definition, nil, report)

	return definitions
}

type resolverDefinitionsVisitor struct {
	*astvisitor.Walker
	definition                 *ast.Document
	resolvers                  *ResolverDefinitions
	dataSourcePlannerFactories []func() DataSourcePlanner
}

func (r *resolverDefinitionsVisitor) EnterFieldDefinition(ref int) {
	for i := 0; i < len(r.dataSourcePlannerFactories); i++ {
		factory := r.dataSourcePlannerFactories[i]
		directiveName := factory().DirectiveName()
		_, exists := r.definition.FieldDefinitionDirectiveByName(ref, directiveName)
		if !exists {
			continue
		}
		*r.resolvers = append(*r.resolvers, DataSourceDefinition{
			TypeName:                 r.definition.FieldDefinitionResolverTypeName(r.EnclosingTypeDefinition),
			FieldName:                r.definition.FieldDefinitionNameBytes(ref),
			DataSourcePlannerFactory: factory,
		})
	}
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
    args: [__InputValue!]!
}

"""
A Directive can be adjacent to many parts of the GraphQL language, a
__DirectiveLocation describes one such possible adjacencies.
"""
enum __DirectiveLocation {
    "Location adjacent to a query operation."
    QUERY
    "Location adjacent to a mutation operation."
    MUTATION
    "Location adjacent to a subscription operation."
    SUBSCRIPTION
    "Location adjacent to a field."
    FIELD
    "Location adjacent to a fragment definition."
    FRAGMENT_DEFINITION
    "Location adjacent to a fragment spread."
    FRAGMENT_SPREAD
    "Location adjacent to an inline fragment."
    INLINE_FRAGMENT
    "Location adjacent to a schema definition."
    SCHEMA
    "Location adjacent to a scalar definition."
    SCALAR
    "Location adjacent to an object type definition."
    OBJECT
    "Location adjacent to a field definition."
    FIELD_DEFINITION
    "Location adjacent to an argument definition."
    ARGUMENT_DEFINITION
    "Location adjacent to an interface definition."
    INTERFACE
    "Location adjacent to a union definition."
    UNION
    "Location adjacent to an enum definition."
    ENUM
    "Location adjacent to an enum value definition."
    ENUM_VALUE
    "Location adjacent to an input object type definition."
    INPUT_OBJECT
    "Location adjacent to an input object field definition."
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
    args: [__InputValue!]!
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
