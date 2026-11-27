package plan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestContextSiblingsUseIndependentFetches(t *testing.T) {
	const schema = `type Query { account: Account }
type Account { locale: [String!] region: [String!] wallet: Wallet result: WalletResult }
union WalletResult = Wallet | GuestWallet
type GuestWallet { nickname: String }
type Wallet { id: ID! quote(input: [String]): Quote sameLocale(input: [String]): String strictLocale(input: [String!]): String regionQuote(input: [String]): String publicValue: String }
type Quote { amount: String }`
	for _, selections := range []string{
		"wallet { q:quote { amount } sameLocale strictLocale r:regionQuote publicValue }",
		"wallet { publicValue r:regionQuote strictLocale sameLocale q:quote { amount } }",
		"result { ... on Wallet { q:quote { amount } sameLocale strictLocale r:regionQuote publicValue } ... on GuestWallet { nickname } }",
	} {
		t.Run(selections, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
			operation := unsafeparser.ParseGraphqlDocumentString("{ account { " + selections + " } }")
			behavior := DataSourcePlanningBehavior{MergeAliasedRootNodes: true, AllowPlanningTypeName: true}
			source := dsb().WithBehavior(behavior).Hash(11).Id("source").RootNode("Query", "account").
				ChildNode("Account", "locale", "region", "wallet", "result").ChildNode("Wallet", "id").ChildNode("GuestWallet", "nickname").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "Wallet", SelectionSet: "id", DisableEntityResolver: true}}).
				SchemaMergedWithBase(schema).DS()
			quotes := dsb().WithBehavior(behavior).Hash(22).Id("quotes").RootNode("Wallet", "id", "quote", "regionQuote", "sameLocale", "strictLocale", "publicValue").
				ChildNode("Quote", "amount").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "Wallet", SelectionSet: "id"}}).
				WithMetadata(func(m *FederationMetaData) {
					m.ContextProviders = []ContextProvider{{ContextName: "account", Provider: ConcreteContextProvider{TypeName: "Account"}}}
					for _, arg := range []struct{ field, source string }{{"quote", "locale"}, {"sameLocale", "locale"}, {"strictLocale", "locale"}, {"regionQuote", "region"}} {
						m.ContextArguments = append(m.ContextArguments, ContextArgument{ContextName: "account", TypeName: "Wallet", FieldName: arg.field, ArgumentName: "input", ContextSelection: "{ " + arg.source + " }"})
					}
				}).SchemaMergedWithBase(schema).DS()
			planner, err := NewPlanner(Configuration{DataSources: []DataSource{source, quotes}})
			require.NoError(t, err)
			report := &operationreport.Report{}
			planner.Plan(&operation, &definition, "", report)
			require.False(t, report.HasErrors(), report.Error())
			fieldFetch := map[string]int{}
			for _, p := range planner.planningPathBuilder.visitor.planners {
				if p.DataSourceConfiguration().Id() != "quotes" {
					continue
				}
				p.ForEachPath(func(path *pathConfiguration) bool {
					if path.pathType == PathTypeField {
						fieldFetch[operation.FieldNameString(path.fieldRef)] = p.ObjectFetchConfiguration().fetchID
					}
					return false
				})
			}
			for _, field := range []string{"quote", "sameLocale", "amount", "regionQuote", "strictLocale", "publicValue"} {
				require.Contains(t, fieldFetch, field)
			}
			require.Equal(t, fieldFetch["quote"], fieldFetch["amount"])
			require.Equal(t, fieldFetch["quote"], fieldFetch["sameLocale"])
			require.NotEqual(t, fieldFetch["quote"], fieldFetch["strictLocale"])
			require.NotEqual(t, fieldFetch["quote"], fieldFetch["regionQuote"])
			require.NotEqual(t, fieldFetch["quote"], fieldFetch["publicValue"])
			require.NotEqual(t, fieldFetch["regionQuote"], fieldFetch["publicValue"])
		})
	}
}
