package plan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestContextOwnerRequiresReachableProjection(t *testing.T) {
	const schema = `type Query { account(n: Int): Account }
type Account { id: ID! token: ID! locale: String wallet: Wallet! }
type Wallet { id: ID! quote: String sku: String }`
	for _, tc := range []struct {
		key, want string
		count     int
	}{{"token", "ordinary", 1}, {"id", "contextual", 1}, {"token", "ordinary", 128}} {
		t.Run(fmt.Sprintf("%s/%d", tc.key, tc.count), func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
			source := dsb().Hash(11).Id("source").RootNode("Query", "account").
				RootNode("Account", "id", "wallet").ChildNode("Wallet", "id").
				KeysMetadata(FederationFieldConfigurations{
					{TypeName: "Account", SelectionSet: "id"},
					{TypeName: "Wallet", SelectionSet: "id", DisableEntityResolver: true},
				}).SchemaMergedWithBase(schema).DS()
			contextual := dsb().Hash(22).Id("contextual").RootNode("Account", tc.key, "locale").
				RootNode("Wallet", "id", "quote", "sku").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "Account", SelectionSet: tc.key}, {TypeName: "Wallet", SelectionSet: "id"}}).
				WithMetadata(func(m *FederationMetaData) {
					m.ContextProviders = []ContextProvider{{ContextName: "account", Provider: ConcreteContextProvider{TypeName: "Account"}}}
					m.ContextArguments = []ContextArgument{{TypeName: "Wallet", FieldName: "quote", ContextName: "account", ContextSelection: "{ locale }"}}
				}).SchemaMergedWithBase(schema).DS()
			ordinary := dsb().Hash(33).Id("ordinary").RootNode("Wallet", "id", "quote").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "Wallet", SelectionSet: "id"}}).SchemaMergedWithBase(schema).DS()
			var operationText strings.Builder
			operationText.WriteString("{")
			for i := 0; i < tc.count; i++ {
				fmt.Fprintf(&operationText, "a%d:account(n:%d){wallet{sku quote}} ", i, i)
			}
			operationText.WriteString("}")
			operation := unsafeparser.ParseGraphqlDocumentString(operationText.String())
			report := &operationreport.Report{}
			builder := NewNodeSelectionBuilder(&Configuration{DataSources: []DataSource{source, contextual, ordinary}})
			builder.ResetSkipFieldRefs()
			result := builder.SelectNodes(&operation, &definition, report)
			require.False(t, report.HasErrors(), report.Error())
			for i := 0; i < tc.count; i++ {
				selected, ok := result.nodeSuggestions.SelectedSuggestionForPath("Wallet", "quote", fmt.Sprintf("query.a%d.wallet.quote", i))
				require.True(t, ok)
				require.Equal(t, tc.want, selected.DataSourceID)
			}
		})
	}
}

func TestContextProjectionRetriesRequiredFieldOwner(t *testing.T) {
	const schema = `type Query { account: Account }
type Account { id: ID! token: ID! locale: String secret: String wallet: Wallet! }
type Wallet { id: ID! quote: String }`
	requiresSecret := func(m *FederationMetaData) {
		m.Requires = FederationFieldConfigurations{{TypeName: "Account", FieldName: "locale", SelectionSet: "secret"}}
	}
	for _, tc := range []struct {
		name, key       string
		configureSource func(*FederationMetaData)
		secretRequires  string
	}{
		{name: "unreachable key", key: "token", configureSource: requiresSecret},
		{name: "cycle through requires", key: "id", configureSource: requiresSecret, secretRequires: "locale"},
		{name: "cycle through context", key: "id", configureSource: func(m *FederationMetaData) {
			m.ContextProviders = []ContextProvider{{ContextName: "root", Provider: ConcreteContextProvider{TypeName: "Query"}}}
			m.ContextArguments = []ContextArgument{{TypeName: "Account", FieldName: "locale", ContextName: "root", ContextSelection: "{ account { locale } }"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
			source := dsb().Hash(11).Id("source").RootNode("Query", "account").
				RootNode("Account", "id", "locale", "wallet").ChildNode("Wallet", "id").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "Account", SelectionSet: "id"}, {TypeName: "Wallet", SelectionSet: "id", DisableEntityResolver: true}}).
				WithMetadata(tc.configureSource).SchemaMergedWithBase(schema).DS()
			contextual := dsb().Hash(22).Id("contextual").RootNode("Account", tc.key, "secret").RootNode("Wallet", "id", "quote").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "Account", SelectionSet: tc.key}, {TypeName: "Wallet", SelectionSet: "id"}}).
				WithMetadata(func(m *FederationMetaData) {
					m.ContextProviders = []ContextProvider{{ContextName: "account", Provider: ConcreteContextProvider{TypeName: "Account"}}}
					m.ContextArguments = []ContextArgument{{TypeName: "Wallet", FieldName: "quote", ContextName: "account", ContextSelection: "{ locale }"}}
					if tc.secretRequires != "" {
						m.Requires = FederationFieldConfigurations{{TypeName: "Account", FieldName: "secret", SelectionSet: tc.secretRequires}}
					}
				}).SchemaMergedWithBase(schema).DS()
			ordinary := dsb().Hash(33).Id("ordinary").RootNode("Account", "id", "locale").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "Account", SelectionSet: "id"}}).SchemaMergedWithBase(schema).DS()
			operation := unsafeparser.ParseGraphqlDocumentString(`{account {wallet {quote}}}`)
			report := &operationreport.Report{}
			builder := NewNodeSelectionBuilder(&Configuration{DataSources: []DataSource{source, contextual, ordinary}})
			builder.ResetSkipFieldRefs()
			result := builder.SelectNodes(&operation, &definition, report)
			require.False(t, report.HasErrors(), report.Error())
			owner, ok := result.nodeSuggestions.SelectedSuggestionForPath("Account", "locale", "query.account.locale")
			require.True(t, ok)
			require.Equal(t, "ordinary", owner.DataSourceID)
		})
	}
}
