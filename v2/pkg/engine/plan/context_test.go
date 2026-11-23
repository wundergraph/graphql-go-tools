package plan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestContextProviderTypes(t *testing.T) {
	schema, report := astparser.ParseGraphqlDocumentString(`
 interface Account { id: ID! }
 type Team implements Account {id:ID!}
 type Person implements Account {id:ID!}
 union Result = Team | Person
 `)
	require.False(t, report.HasErrors())
	provider := []ContextProvider{{ContextName: "locale", TypeName: "Account", ConcreteTypeNames: []string{"Team"}}}
	metadata := FederationMetaData{ContextProviders: provider}
	matches, all := metadata.providerTypes(&schema, "locale", "Result")
	require.ElementsMatch(t, []string{"Person", "Team"}, matches)
	require.True(t, all)
	metadata.ContextProviders = []ContextProvider{{ContextName: "locale", TypeName: "Person"}}
	matches, all = metadata.providerTypes(&schema, "locale", "Account")
	require.Equal(t, []string{"Person"}, matches)
	require.False(t, all)
}

func TestContextSelectionValues(t *testing.T) {
	schema, report := astparser.ParseGraphqlDocumentString(`type Account { formatted(upper: Boolean!): String }`)
	require.False(t, report.HasErrors())
	planner := nodeSelectionVisitor{definition: &schema}
	fragment, reportPtr := RequiredFieldsFragment("Account", `formatted(upper: true)`, false)
	require.False(t, reportPtr.HasErrors())
	set := fragment.FragmentDefinitions[0].SelectionSet
	field, err := contextFieldsForType(&schema, fragment, set, "Account")
	require.NoError(t, err)
	require.Equal(t, "formatted(upper: true)", field)
	fragment, reportPtr = RequiredFieldsFragment("Account", "__typename", false)
	require.False(t, reportPtr.HasErrors())
	require.Equal(t, []resolve.ContextSelection{{Field: "__typename"}}, planner.contextSelection(fragment, fragment.FragmentDefinitions[0].SelectionSet, "Account", nil))
}

func TestContextSelectionForRootAndInterfaceProvider(t *testing.T) {
	schema, report := astparser.ParseGraphqlDocumentString(`
		scalar String
		scalar ID
		type Query { source: Source }
		interface Source { wallet(group: String): Wallet }
		type Mutation implements Source { wallet(group: String = "active"): Wallet }
		type Remote implements Source { wallet(group: String): Wallet }
		type Wallet { id: ID }
	`)
	require.False(t, report.HasErrors())
	selection, errors := RequiredFieldsFragment("Source",
		`... on Mutation {wallet {id}} ... on Source {wallet(group:"active") {id}}`, false)
	require.False(t, errors.HasErrors())
	set := selection.FragmentDefinitions[0].SelectionSet
	field, err := contextFieldsForType(&schema, selection, set, "Mutation")
	require.NoError(t, err)
	require.Equal(t, "wallet { id }", field)
	field, err = contextFieldsForType(&schema, selection, set, "Remote")
	require.NoError(t, err)
	require.Equal(t, `wallet(group: "active") { id }`, field)
	// A client can select the shared field directly on Source. The root-only
	// override would require an impossible mutation field on a remote owner.
	field, err = contextFieldsForType(&schema, selection, set, "Source")
	require.NoError(t, err)
	require.Equal(t, `wallet(group: "active") { id }`, field)

	op, parsed := astparser.ParseGraphqlDocumentString("query {source {__typename}}")
	require.False(t, parsed.HasErrors())
	rootSet := op.OperationDefinitions[0].SelectionSet
	root := op.Selections[op.SelectionSets[rootSet].SelectionRefs[0]].Ref
	payload := op.Fields[root].SelectionSet
	walker := astvisitor.NewWalker(4)
	walker.Ancestors = []ast.Node{{Kind: ast.NodeKindOperationDefinition}, {Kind: ast.NodeKindSelectionSet, Ref: rootSet}, {Kind: ast.NodeKindField, Ref: root}}
	planner := nodeSelectionVisitor{operation: &op, definition: &schema, walker: &walker, selectionSetRefs: []int{rootSet, payload}}
	source, _, err := planner.addContextSource(payload, contextScope{typeName: "Source", depth: 1}, []string{"Mutation", "Remote"}, selection, fieldRequirementsContext{})
	require.NoError(t, err)
	require.Equal(t, []resolve.ContextSelection{{Field: "wallet", Children: []resolve.ContextSelection{{Field: "id"}}}}, source.Selection)
	for _, ref := range op.SelectionSets[payload].SelectionRefs {
		require.Equal(t, ast.SelectionKindField, op.Selections[ref].Kind)
	}
}

func TestContextualResolverEligibility(t *testing.T) {
	schema, report := astparser.ParseGraphqlDocumentString(`
		type Query { accounts: [Account] }
		interface Account { wallet: Wallet }
		type Member implements Account { wallet: Wallet }
		type Team implements Account { wallet: Wallet }
		type Wallet { quote: String }
	`)
	require.False(t, report.HasErrors())
	metadata := FederationMetaData{
		ContextProviders: []ContextProvider{{ContextName: "owner", TypeName: "Member"}},
		ContextArguments: []ContextArgument{{ContextName: "owner", TypeName: "Wallet", FieldName: "quote"}},
	}
	collector := dsNodesCollector{definition: &schema, contextMetadata: &metadata, info: map[int]fieldInfo{
		0: {typeName: "Query", parentFieldRef: ast.InvalidRef},
		1: {typeName: "Account", parentFieldRef: 0},
		2: {typeName: "Member", parentFieldRef: 0},
		3: {typeName: "Team", parentFieldRef: 0},
	}}
	quote := fieldInfo{typeName: "Wallet", fieldName: "quote", parentFieldRef: 1}
	require.False(t, collector.hasRequiredContexts(quote))
	require.Equal(t, []int{0}, collector.partialContextScopes)
	quote.parentFieldRef = 2
	require.True(t, collector.hasRequiredContexts(quote))
	quote.parentFieldRef = 3
	require.False(t, collector.hasRequiredContexts(quote))
}

func TestOperationRootContextEligibility(t *testing.T) {
	for _, operation := range []string{"mutation", "subscription"} {
		t.Run(operation, func(t *testing.T) {
			schema, report := astparser.ParseGraphqlDocumentString(`
		schema { query: Query ` + operation + `: Events }
		type Query { ping: Boolean }
		input Filter { ids: [ID!]! weight: Float = 2.5 }
		type Events { wallet(tag: String, group: String = "active", filter: Filter = {ids: ["1"]}): Wallet stamp: ID }
		type Wallet { id: ID! quote: ID }
	`)
			require.False(t, report.HasErrors())
			for _, tt := range []struct {
				name, trigger, selection string
				want                     bool
			}{
				{"active trigger", "event:wallet", "{ wallet { id } }", true},
				{"unrelated stream", "event:wallet", "{ stamp }", false},
				{"aliased trigger and nested fragment", "event:wallet", "{ ... on Events { wallet { id } } }", true},
				{"constant typename", "event:wallet", "{ __typename }", true},
				{"explicit default", "event:wallet", `{ wallet(group:"active") { id } }`, true},
				{"different from default", "event:wallet", `{ wallet(group:"other") { id } }`, false},
				{"literal input object default", "event:wallet", `{ wallet(filter:{ids:["1"]}) { id } }`, true},
				{"undeclared argument cannot match omission", "event:wallet", `{ wallet(missing:1) { id } }`, false},
				{"distinct nested input object", "event:wallet", `{ wallet(filter:{weight:2.50, ids:2}) { id } }`, false},
				{"no schema default for omitted argument", "event:wallet", `{ wallet(tag:null) { id } }`, false},
				{"matching arguments", "event:wallet(tag:\"a\")", "{ wallet(tag:\"a\") { id } }", true},
				{"different arguments", "event:wallet(tag:\"a\")", "{ wallet(tag:\"b\") { id } }", false},
			} {
				t.Run(tt.name, func(t *testing.T) {
					op, report := astparser.ParseGraphqlDocumentString(operation + " {" + tt.trigger + "{quote}}")
					require.False(t, report.HasErrors())
					rootField := op.Selections[op.SelectionSets[op.OperationDefinitions[0].SelectionSet].SelectionRefs[0]].Ref
					metadata := FederationMetaData{
						ContextProviders: []ContextProvider{{ContextName: "event", TypeName: "Events"}},
						ContextArguments: []ContextArgument{{ContextName: "event", TypeName: "Wallet", FieldName: "quote", ContextSelection: tt.selection}},
					}
					collector := dsNodesCollector{operation: &op, definition: &schema, contextMetadata: &metadata, info: map[int]fieldInfo{
						rootField: {typeName: "Events", fieldName: "wallet", parentFieldRef: ast.InvalidRef},
					}}
					require.Equal(t, tt.want, collector.hasRequiredContexts(fieldInfo{typeName: "Wallet", fieldName: "quote", parentFieldRef: rootField}))
				})
			}
		})
	}
}

func TestPlanContextOnOperationRoot(t *testing.T) {
	schema, report := astparser.ParseGraphqlDocumentString(`
		scalar String
		scalar ID
		schema { query: Query mutation: Change subscription: Events }
		type Query { wallet: Wallet }
		type Change { wallet: Wallet }
		type Events { wallet: Wallet }
		type Wallet { id: ID! quote: ID }
	`)
	require.False(t, report.HasErrors())
	for _, tt := range []struct {
		operation, root, selection string
		want                       resolve.ContextSource
	}{
		{"subscription", "Events", "wallet { id }", resolve.ContextSource{Selection: []resolve.ContextSelection{{Field: "event", Children: []resolve.ContextSelection{{Field: "id"}}}}}},
		{"mutation", "Change", "wallet { id }", resolve.ContextSource{Selection: []resolve.ContextSelection{{Field: "event", Children: []resolve.ContextSelection{{Field: "id"}}}}}},
		{"subscription", "Events", "__typename", resolve.ContextSource{RootTypeName: "Events"}},
		{"mutation", "Change", "__typename", resolve.ContextSource{RootTypeName: "Change"}},
		{"query", "Query", "__typename", resolve.ContextSource{RootTypeName: "Query"}},
	} {
		t.Run(tt.operation+"/"+tt.selection, func(t *testing.T) {
			op, report := astparser.ParseGraphqlDocumentString(tt.operation + " { event:wallet { quote } }")
			require.False(t, report.HasErrors())
			rootSet := op.OperationDefinitions[0].SelectionSet
			trigger := op.Selections[op.SelectionSets[rootSet].SelectionRefs[0]].Ref
			payload := op.Fields[trigger].SelectionSet
			walker := astvisitor.NewWalker(4)
			walker.Ancestors = []ast.Node{{Kind: ast.NodeKindOperationDefinition}, {Kind: ast.NodeKindSelectionSet, Ref: rootSet}, {Kind: ast.NodeKindField, Ref: trigger}}
			planner := nodeSelectionVisitor{definition: &schema, operation: &op, walker: &walker, selectionSetRefs: []int{rootSet, payload}}
			fragment, reportPtr := RequiredFieldsFragment(tt.root, tt.selection, false)
			require.False(t, reportPtr.HasErrors())
			source, _, err := planner.addContextSource(rootSet, contextScope{typeName: tt.root}, []string{tt.root}, fragment, fieldRequirementsContext{})
			require.NoError(t, err)
			require.Equal(t, tt.want, source)
			require.Len(t, op.SelectionSets[rootSet].SelectionRefs, 1)
			if tt.selection != "__typename" {
				found, _ := op.SelectionSetHasFieldSelectionWithExactName(payload, []byte("id"))
				require.True(t, found)
			}
		})
	}
}

func TestContextDependencyCoexistsWithRequires(t *testing.T) {
	const schema = `type Query { account: Account }
	type Account { id: ID! locale: String score: Int wallet: Wallet! }
	type Wallet { id: ID! quote: String }`
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
	accounts := dsb().Hash(11).Id("accounts").RootNode("Query", "account").
		RootNode("Account", "id", "locale", "wallet").ChildNode("Wallet", "id").
		KeysMetadata(FederationFieldConfigurations{{TypeName: "Account", SelectionSet: "id"}, {TypeName: "Wallet", SelectionSet: "id", DisableEntityResolver: true}}).SchemaMergedWithBase(schema).DS()
	quotes := dsb().Hash(22).Id("quotes").RootNode("Account", "id", "score").RootNode("Wallet", "id", "quote").
		KeysMetadata(FederationFieldConfigurations{{TypeName: "Account", SelectionSet: "id"}, {TypeName: "Wallet", SelectionSet: "id"}}).
		WithMetadata(func(m *FederationMetaData) {
			m.Requires = FederationFieldConfigurations{{TypeName: "Account", FieldName: "score", SelectionSet: "locale"}}
			m.ContextProviders = []ContextProvider{{ContextName: "account", TypeName: "Account"}}
			m.ContextArguments = []ContextArgument{{TypeName: "Wallet", FieldName: "quote", ContextName: "account", ContextSelection: "{ locale }"}}
		}).SchemaMergedWithBase(schema).DS()
	op := unsafeparser.ParseGraphqlDocumentString(`{account {locale score wallet {quote}}}`)
	report := &operationreport.Report{}
	builder := NewNodeSelectionBuilder(&Configuration{DataSources: []DataSource{accounts, quotes}})
	builder.ResetSkipFieldRefs()
	result := builder.SelectNodes(&op, &definition, report)
	require.False(t, report.HasErrors(), report.Error())
	var checked []string
	for key, kind := range result.fieldDependencyKind {
		if op.FieldNameString(key.dependsOn) != "locale" {
			continue
		}
		require.NotContains(t, result.skipFieldsRefs, key.dependsOn, "keep the publicly selected locale")
		field := op.FieldNameString(key.field)
		switch field {
		case "score":
			require.Equal(t, fieldDependencyKindRequires, kind)
		case "quote":
			require.Equal(t, fieldDependencyKindContext, kind)
		}
		checked = append(checked, field)
	}
	require.ElementsMatch(t, []string{"quote", "score"}, checked)
}
