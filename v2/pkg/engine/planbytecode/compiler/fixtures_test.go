package compiler

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func syncResponsePlan(fetches *resolve.FetchTreeNode, data *resolve.Object) *plan.SynchronousResponsePlan {
	return &plan.SynchronousResponsePlan{
		Response: graphqlResponse(fetches, data),
	}
}

func graphqlResponse(fetches *resolve.FetchTreeNode, data *resolve.Object) *resolve.GraphQLResponse {
	return &resolve.GraphQLResponse{
		Info:    &resolve.GraphQLResponseInfo{},
		Fetches: fetches,
		Data:    data,
	}
}

func rootObject(fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{Fields: fields}
}

func objectValue(path string, fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{
		Path:   pathOf(path),
		Fields: fields,
	}
}

func arrayValue(path string, item resolve.Node) *resolve.Array {
	return &resolve.Array{
		Path: pathOf(path),
		Item: item,
	}
}

func field(name string, value resolve.Node) *resolve.Field {
	return &resolve.Field{
		Name:  []byte(name),
		Value: value,
	}
}

func stringValue(path string, nullable ...bool) *resolve.String {
	return &resolve.String{
		Path:     pathOf(path),
		Nullable: optionalBool(nullable),
	}
}

func integerValue(path string, nullable ...bool) *resolve.Integer {
	return &resolve.Integer{
		Path:     pathOf(path),
		Nullable: optionalBool(nullable),
	}
}

func booleanValue(path string, nullable ...bool) *resolve.Boolean {
	return &resolve.Boolean{
		Path:     pathOf(path),
		Nullable: optionalBool(nullable),
	}
}

func staticStringValue(value string) *resolve.StaticString {
	return &resolve.StaticString{Value: value}
}

func staticStringAt(path, value string) *resolve.StaticString {
	return &resolve.StaticString{
		Path:  pathOf(path),
		Value: value,
	}
}

func batchEntityFetch(subgraphName string, selectDataPath ...string) *resolve.BatchEntityFetch {
	return &resolve.BatchEntityFetch{
		PostProcessing: resolve.PostProcessingConfiguration{
			SelectResponseDataPath: selectDataPath,
		},
		Info: &resolve.FetchInfo{
			DataSourceName: subgraphName,
		},
	}
}

func entityFetch(subgraphName string, selectDataPath ...string) *resolve.EntityFetch {
	return &resolve.EntityFetch{
		PostProcessing: resolve.PostProcessingConfiguration{
			SelectResponseDataPath: selectDataPath,
		},
		Info: &resolve.FetchInfo{
			DataSourceName: subgraphName,
		},
	}
}

func singleFetch(subgraphName, query string, selectDataPath, mergePath []string) *resolve.SingleFetch {
	return &resolve.SingleFetch{
		FetchConfiguration: resolve.FetchConfiguration{
			PostProcessing: resolve.PostProcessingConfiguration{
				SelectResponseDataPath:   selectDataPath,
				SelectResponseErrorsPath: []string{"errors"},
				MergePath:                mergePath,
			},
		},
		FetchDependencies: resolve.FetchDependencies{FetchID: 1},
		Info: &resolve.FetchInfo{
			DataSourceID:   subgraphName + "-id",
			DataSourceName: subgraphName,
			QueryPlan: &resolve.QueryPlan{
				Query: query,
			},
		},
	}
}

func pathOf(path string) []string {
	if path == "" {
		return nil
	}
	return []string{path}
}

func optionalBool(values []bool) bool {
	return len(values) != 0 && values[0]
}
