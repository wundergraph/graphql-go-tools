package execution

import (
	"github.com/gobuffalo/packr"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"go.uber.org/zap"
	"io"
)

type DataSource interface {
	Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction
}

type DataSourcePlanner interface {
	// DirectiveName is the @directive associated with this DataSource
	// This value is important to tie the directive to the DataSourcePlanner
	// The value is case sensitive
	DirectiveName() []byte
	/*
		DirectiveDefinition defines the specific directive for the DataSource in GraphQL SDL language
		Example:

		"""
		HttpDataSource
		"""
		directive @HttpDataSource (
			"""
			host is the host name of the data source, e.g. example.com
			"""
			host: String!
		) on FIELD_DEFINITION
	*/
	DirectiveDefinition() []byte
	// Plan should return the instantiated DataSource and the Arguments accordingly
	// You usually want to take the prepared Arguments from Initialize(...), append or prepend your custom args and then return it in Plan(...)
	// Keep in mind that not returning the Arguments from Initialize(...) will probably break something
	Plan() (DataSource, []Argument)
	// Initialize is the function to initialize all important values for the DataSourcePlanner to function correctly
	// You probably need access to the walker, operation and definition to use the DataSourcePlanner to its full power
	// walker gives you useful information from within all visitor Callbacks, e.g. the Path & Ancestors
	// operation is the AST of the GraphQL operation
	// definition is the AST of the GraphQL schema definition
	// args are the pre-calculated Arguments from the planner
	// resolverParameters are the parameters from the @directive params field
	Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter)
	/*
		OverrideRootPathSelector gives the DataSourcePlanner the capability to change the path of the root field
		Example:

		HTTP API response for /foo:
		{ "bar": "baz" }

		GraphQL API response { foo { bar } }:
		{ "foo": { "bar": "baz" } }

		As you can see, the HTTP API response is a flat representation of the resource.
		The GraphQL API response is embedded in a "foo" object. (Correctly, foo would be embedded in a "data" object but this is neglectable.)
		Therefore if you want to extract the data from the response you need to return the correct path.
		For the GraphQL API the correct root field path is "foo", for the HTTP API its "" (empty string).
		The path will be generated by the planner, however the planner doesn't know that the HTTP API response is flat.
		Therefore in case of the HTTP API we need to override the path to be nil and leave the path as is for GraphQL APIs.
	*/
	astvisitor.EnterInlineFragmentVisitor
	astvisitor.LeaveInlineFragmentVisitor
	astvisitor.EnterSelectionSetVisitor
	astvisitor.LeaveSelectionSetVisitor
	astvisitor.EnterFieldVisitor
	astvisitor.LeaveFieldVisitor
}

type BaseDataSourcePlanner struct {
	log                   *zap.Logger
	walker                *astvisitor.Walker // nolint
	definition, operation *ast.Document      // nolint
	args                  []Argument         // nolint
	graphqlDefinitions    *packr.Box         // nolint
	rootField             rootField          // nolint
}

type rootField struct {
	isDefined bool
	ref       int
}

func (r *rootField) setIfNotDefined(ref int){
	if r.isDefined {
		return
	}
	r.isDefined = true
	r.ref = ref
}

func (r *rootField) isDefinedAndEquals(ref int) bool {
	return r.isDefined && r.ref == ref
}