package plan

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestPlannerMultiHopCompoundKey(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentString(multiHopCompoundKeyDefinition)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))

	operation := unsafeparser.ParseGraphqlDocumentString(`query {
		topProducts {
			first { id }
			selected { id }
		}
	}`)

	report := &operationreport.Report{}
	astnormalization.NewNormalizer(true, true).NormalizeOperation(&operation, &definition, report)
	require.False(t, report.HasErrors(), report.Error())

	astvalidation.DefaultOperationValidator().Validate(&operation, &definition, report)
	require.False(t, report.HasErrors(), report.Error())

	planner, err := NewPlanner(Configuration{
		DataSources:                     multiHopCompoundKeyDataSources(),
		DisableIncludeInfo:              true,
		DisableIncludeFieldDependencies: true,
	})
	require.NoError(t, err)

	plan := planner.Plan(&operation, &definition, "", report)
	require.False(t, report.HasErrors(), report.Error())
	require.Equal(t, strings.TrimSpace(multiHopCompoundKeyExpectedPlan), strings.TrimSpace(planString(plan)))
}

func planString(v any) string {
	formatterConfig := map[reflect.Type]any{
		reflect.TypeFor[[]byte](): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
		reflect.TypeOf(map[string]struct{}{}): func(m map[string]struct{}) string {
			var keys []string
			for k := range m {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			return fmt.Sprintf("%q", keys)
		},
	}

	prettyCfg := &pretty.Config{
		Diffable:          true,
		IncludeUnexported: false,
		Formatter:         formatterConfig,
	}

	return prettyCfg.Sprint(v)
}

func multiHopCompoundKeyDataSources() []DataSource {
	return []DataSource{
		dsb().
			WithBehavior(multiHopCompoundKeyPlanningBehavior()).
			Hash(11).
			Id("catalog").
			RootNode("Query", "topProducts").
			RootNode("ProductList", "products").
			RootNode("Product", "id", "category").
			RootNode("Category", "mainProduct", "id", "tag").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "ProductList", SelectionSet: "products { id }"},
				{TypeName: "Product", SelectionSet: "id"},
				{TypeName: "Category", SelectionSet: "id"},
				{TypeName: "Category", SelectionSet: "id tag", DisableEntityResolver: true},
			}).
			SchemaMergedWithBase(multiHopCatalogSubgraphSchema).
			DS(),
		dsb().
			WithBehavior(multiHopCompoundKeyPlanningBehavior()).
			Hash(22).
			Id("link").
			RootNode("Product", "id", "pid").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "Product", SelectionSet: "id"},
				{TypeName: "Product", SelectionSet: "id pid"},
			}).
			SchemaMergedWithBase(multiHopLinkSubgraphSchema).
			DS(),
		dsb().
			WithBehavior(multiHopCompoundKeyPlanningBehavior()).
			Hash(33).
			Id("collection").
			RootNode("ProductList", "products", "first", "selected").
			RootNode("Product", "id", "pid").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "ProductList", SelectionSet: "products { id pid }"},
				{TypeName: "ProductList", SelectionSet: "products { id }", DisableEntityResolver: true},
				{TypeName: "Product", SelectionSet: "id pid"},
				{TypeName: "Product", SelectionSet: "id", DisableEntityResolver: true},
			}).
			SchemaMergedWithBase(multiHopCollectionSubgraphSchema).
			DS(),
		dsb().
			WithBehavior(multiHopCompoundKeyPlanningBehavior()).
			Hash(44).
			Id("pricing").
			RootNode("ProductList", "products", "first", "selected").
			RootNode("Product", "id", "price", "pid", "category").
			RootNode("Category", "id", "tag").
			ChildNode("Price", "price").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "ProductList", SelectionSet: "products { category { id tag } id pid } selected { id }"},
				{TypeName: "ProductList", SelectionSet: "products { id }", DisableEntityResolver: true},
				{TypeName: "ProductList", SelectionSet: "products { id pid }", DisableEntityResolver: true},
				{TypeName: "Product", SelectionSet: "category { id tag } id pid"},
				{TypeName: "Product", SelectionSet: "id", DisableEntityResolver: true},
				{TypeName: "Product", SelectionSet: "id pid", DisableEntityResolver: true},
				{TypeName: "Category", SelectionSet: "id tag"},
				{TypeName: "Category", SelectionSet: "id", DisableEntityResolver: true},
			}).
			SchemaMergedWithBase(multiHopPricingSubgraphSchema).
			DS(),
	}
}

func multiHopCompoundKeyPlanningBehavior() DataSourcePlanningBehavior {
	return DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      true,
		OverrideFieldPathFromAlias: true,
		AllowPlanningTypeName:      true,
	}
}

const multiHopCompoundKeyDefinition = `
type Query {
	topProducts: ProductList!
}

type ProductList {
	products: [Product!]!
	first: Product
	selected: Product
}

type Product {
	id: ID!
	pid: ID
	category: Category
	price: Price
}

type Category {
	mainProduct: Product!
	id: ID!
	tag: String!
}

type Price {
	price: Float!
}
`

const multiHopCatalogSubgraphSchema = `
type Query {
	topProducts: ProductList!
}

type ProductList {
	products: [Product!]!
}

type Product {
	id: ID!
	category: Category
}

type Category {
	mainProduct: Product!
	id: ID!
	tag: String!
}
`

const multiHopCollectionSubgraphSchema = `
type ProductList {
	products: [Product!]!
	first: Product
	selected: Product
}

type Product {
	id: ID!
	pid: ID!
}
`

const multiHopLinkSubgraphSchema = `
type Product {
	id: ID!
	pid: ID!
}
`

const multiHopPricingSubgraphSchema = `
type ProductList {
	products: [Product!]!
	selected: Product
}

type Product {
	id: ID!
	price: Price
	pid: ID!
	category: Category
}

type Category {
	id: ID!
	tag: String!
}

type Price {
	price: Float!
}
`

const multiHopCompoundKeyExpectedPlan = `
{
 Response: {
  Data: {
   Nullable: false,
   Path: [
   ],
   Fields: [
    {
     Name: "topProducts",
     Value: {
      Nullable: false,
      Path: [
       "topProducts",
      ],
      Fields: [
       {
        Name: "first",
        Value: {
         Nullable: true,
         Path: [
          "first",
         ],
         Fields: [
          {
           Name: "id",
           Value: {
            Path: [
             "id",
            ],
            Nullable: false,
            Export: nil,
           },
           Position: {
            Line: 3,
            Column: 12,
           },
           Defer: nil,
           Stream: nil,
           OnTypeNames: [
           ],
           ParentOnTypeNames: [
           ],
           Info: nil,
          },
         ],
         Unresolvable: false,
         PossibleTypes: ["Product"],
         SourceName: "",
         TypeName: "Product",
        },
        Position: {
         Line: 3,
         Column: 4,
        },
        Defer: nil,
        Stream: nil,
        OnTypeNames: [
        ],
        ParentOnTypeNames: [
        ],
        Info: nil,
       },
       {
        Name: "selected",
        Value: {
         Nullable: true,
         Path: [
          "selected",
         ],
         Fields: [
          {
           Name: "id",
           Value: {
            Path: [
             "id",
            ],
            Nullable: false,
            Export: nil,
           },
           Position: {
            Line: 4,
            Column: 15,
           },
           Defer: nil,
           Stream: nil,
           OnTypeNames: [
           ],
           ParentOnTypeNames: [
           ],
           Info: nil,
          },
         ],
         Unresolvable: false,
         PossibleTypes: ["Product"],
         SourceName: "",
         TypeName: "Product",
        },
        Position: {
         Line: 4,
         Column: 4,
        },
        Defer: nil,
        Stream: nil,
        OnTypeNames: [
        ],
        ParentOnTypeNames: [
        ],
        Info: nil,
       },
      ],
      Unresolvable: false,
      PossibleTypes: ["ProductList"],
      SourceName: "",
      TypeName: "ProductList",
     },
     Position: {
      Line: 2,
      Column: 3,
     },
     Defer: nil,
     Stream: nil,
     OnTypeNames: [
     ],
     ParentOnTypeNames: [
     ],
     Info: nil,
    },
   ],
   Unresolvable: false,
   PossibleTypes: [],
   SourceName: "",
   TypeName: "",
  },
  RawFetches: [
   {
    Fetch: {
     FetchConfiguration: {
      Input: "",
      Variables: [
      ],
      DataSource: {
      },
      RequiresEntityFetch: false,
      RequiresEntityBatchFetch: false,
      PostProcessing: {
       SelectResponseDataPath: [
       ],
       SelectResponseErrorsPath: [
       ],
       MergePath: [
       ],
      },
      SetTemplateOutputToNullOnVariableNull: false,
      QueryPlan: nil,
      OperationName: "",
     },
     FetchDependencies: {
      FetchID: 0,
      DependsOnFetchIDs: [
      ],
      DeferID: 0,
     },
     InputTemplate: {
      Segments: [
      ],
      SetTemplateOutputToNullOnVariableNull: false,
     },
     DataSourceIdentifier: "plan.FakeDataSource",
     Trace: nil,
     Info: nil,
    },
    FetchPath: [
    ],
    ResponsePath: "",
    ResponsePathElements: [
    ],
   },
   {
    Fetch: {
     FetchConfiguration: {
      Input: "",
      Variables: [
      ],
      DataSource: {
      },
      RequiresEntityFetch: false,
      RequiresEntityBatchFetch: false,
      PostProcessing: {
       SelectResponseDataPath: [
       ],
       SelectResponseErrorsPath: [
       ],
       MergePath: [
       ],
      },
      SetTemplateOutputToNullOnVariableNull: false,
      QueryPlan: nil,
      OperationName: "",
     },
     FetchDependencies: {
      FetchID: 1,
      DependsOnFetchIDs: [
       0,
      ],
      DeferID: 0,
     },
     InputTemplate: {
      Segments: [
      ],
      SetTemplateOutputToNullOnVariableNull: false,
     },
     DataSourceIdentifier: "plan.FakeDataSource",
     Trace: nil,
     Info: nil,
    },
    FetchPath: [
     {
      Kind: "object",
      Path: [
       "topProducts",
      ],
      TypeNames: [
      ],
     },
     {
      Kind: "array",
      Path: [
       "products",
      ],
      TypeNames: [
      ],
     },
    ],
    ResponsePath: "topProducts.products",
    ResponsePathElements: [
     "topProducts",
     "products",
    ],
   },
   {
    Fetch: {
     FetchConfiguration: {
      Input: "",
      Variables: [
      ],
      DataSource: {
      },
      RequiresEntityFetch: false,
      RequiresEntityBatchFetch: false,
      PostProcessing: {
       SelectResponseDataPath: [
       ],
       SelectResponseErrorsPath: [
       ],
       MergePath: [
       ],
      },
      SetTemplateOutputToNullOnVariableNull: false,
      QueryPlan: nil,
      OperationName: "",
     },
     FetchDependencies: {
      FetchID: 2,
      DependsOnFetchIDs: [
       0,
       1,
      ],
      DeferID: 0,
     },
     InputTemplate: {
      Segments: [
      ],
      SetTemplateOutputToNullOnVariableNull: false,
     },
     DataSourceIdentifier: "plan.FakeDataSource",
     Trace: nil,
     Info: nil,
    },
    FetchPath: [
     {
      Kind: "object",
      Path: [
       "topProducts",
      ],
      TypeNames: [
      ],
     },
    ],
    ResponsePath: "topProducts",
    ResponsePathElements: [
     "topProducts",
    ],
   },
  ],
  Fetches: nil,
  Info: nil,
  DataSources: [
  ],
 },
 FlushInterval: 0,
 CostCalculator: nil,
}
`
