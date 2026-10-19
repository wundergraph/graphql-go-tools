package resolve

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

func TestResponseCacheShapeValid(t *testing.T) {
	t.Parallel()

	from := func(ids ...string) *FieldInfo { return &FieldInfo{Source: TypeFieldSource{IDs: ids}} }

	// query { employee { id name tags role details { age } } } where details
	// comes from subgraph b.
	plan := &Object{Fields: []*Field{
		{
			Name: []byte("employee"),
			Info: from("a"),
			Value: &Object{Path: []string{"employee"}, Nullable: true, Fields: []*Field{
				{Name: []byte("id"), Info: from("a"), Value: &Integer{Path: []string{"id"}}},
				{Name: []byte("name"), Info: from("a"), Value: &String{Path: []string{"name"}, Nullable: true}},
				{Name: []byte("tags"), Info: from("a"), Value: &Array{Path: []string{"tags"}, Item: &String{}}},
				{Name: []byte("role"), Info: from("a"), Value: &Enum{Path: []string{"role"}, Values: []string{"ADMIN", "USER"}}},
				{Name: []byte("details"), Info: from("b"), Value: &Object{Path: []string{"details"}, Fields: []*Field{
					{Name: []byte("age"), Info: from("b"), Value: &Integer{Path: []string{"age"}}},
				}}},
			}},
		},
		{
			Name: []byte("employees"),
			Info: from("a"),
			Value: &Array{Path: []string{"employees"}, Item: &Object{Fields: []*Field{
				{Name: []byte("id"), Info: from("a"), Value: &Integer{Path: []string{"id"}}},
				{Name: []byte("age"), Info: from("b"), Value: &Integer{Path: []string{"age"}}},
				{Name: []byte("score"), Info: from("b"), OnTypeNames: [][]byte{[]byte("Manager")}, Value: &Float{Path: []string{"score"}}},
			}}},
		},
	}}

	for _, tc := range []struct {
		name  string
		path  []string
		ds    string
		value string
		want  bool
	}{
		{name: "a matching root response", ds: "a", value: `{"employee":{"id":1,"name":"x","tags":["t"],"role":"ADMIN"}}`, want: true},
		{name: "a string where an object belongs", ds: "a", value: `{"employee":"oops"}`},
		{name: "null for a nullable object", ds: "a", value: `{"employee":null}`, want: true},
		{name: "a string where an int belongs", ds: "a", value: `{"employee":{"id":"1"}}`},
		{name: "null for a non-null field", ds: "a", value: `{"employee":{"id":null}}`},
		{name: "null for a nullable field", ds: "a", value: `{"employee":{"id":1,"name":null}}`, want: true},
		{name: "a scalar where a list belongs", ds: "a", value: `{"employee":{"id":1,"tags":"t"}}`},
		{name: "a wrong list item", ds: "a", value: `{"employee":{"id":1,"tags":[1]}}`},
		{name: "an unknown enum value", ds: "a", value: `{"employee":{"id":1,"role":"ROOT"}}`},
		{name: "an absent field passes", ds: "a", value: `{"employee":{}}`, want: true},
		{name: "another subgraph's fields are not checked", ds: "a", value: `{"employee":{"id":1,"details":"oops"}}`, want: true},

		{name: "an entity at an object path", path: []string{"employee"}, ds: "b", value: `{"__typename":"Employee","details":{"age":3}}`, want: true},
		{name: "a wrong entity field", path: []string{"employee"}, ds: "b", value: `{"__typename":"Employee","details":{"age":"3"}}`},
		{name: "an entity in a list", path: []string{"employees", "@"}, ds: "b", value: `{"__typename":"Employee","age":3}`, want: true},
		{name: "a wrong entity field in a list", path: []string{"employees", "@"}, ds: "b", value: `{"__typename":"Employee","age":"3"}`},
		{name: "a fragment field on another type is not checked", path: []string{"employees", "@"}, ds: "b", value: `{"__typename":"Employee","score":"x"}`, want: true},
		{name: "a fragment field on its type is checked", path: []string{"employees", "@"}, ds: "b", value: `{"__typename":"Manager","score":"x"}`},
		{name: "a path outside the plan passes", path: []string{"nowhere"}, ds: "b", value: `{"age":"3"}`, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loader := &Loader{responsePlan: plan}
			got := loader.responseCacheShapesValid(&FetchItem{ResponsePathElements: tc.path}, tc.ds, []*astjson.Value{astjson.MustParse(tc.value)})
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("no plan passes", func(t *testing.T) {
		t.Parallel()
		require.True(t, (&Loader{}).responseCacheShapesValid(&FetchItem{}, "a", []*astjson.Value{astjson.MustParse(`{"employee":"oops"}`)}))
	})
}

func TestResponseCacheCollectValidatesShape(t *testing.T) {
	t.Parallel()

	plan := &Object{Fields: []*Field{{
		Name: []byte("employee"),
		Info: &FieldInfo{Source: TypeFieldSource{IDs: []string{"accounts-id"}}},
		Value: &Object{Path: []string{"employee"}, Nullable: true, Fields: []*Field{
			{Name: []byte("id"), Info: &FieldInfo{Source: TypeFieldSource{IDs: []string{"accounts-id"}}}, Value: &Integer{Path: []string{"id"}}},
		}},
	}}}

	collect := func(t *testing.T, body string, validate bool) *preparedFetch {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.SetResponseCache(ResponseCacheOptions{
			Store:             newTestCache(),
			DefaultTTL:        time.Minute,
			ValidateResponses: validate,
		})
		res := &result{
			out:        []byte(body),
			statusCode: http.StatusOK,
			httpResponseContext: &httpclient.ResponseContext{
				Response: &http.Response{Header: http.Header{"Cache-Control": []string{"max-age=60"}}},
			},
		}
		res.init(PostProcessingConfiguration{
			SelectResponseDataPath:   []string{"data"},
			SelectResponseErrorsPath: []string{"errors"},
		}, &FetchInfo{DataSourceID: "accounts-id", DataSourceName: "accounts"})
		prepared := &preparedFetch{item: &FetchItem{}, res: res, responseCacheKeys: []string{"root"}, isRootFetchCache: true}
		loader := &Loader{ctx: ctx, responsePlan: plan}
		require.NoError(t, loader.responseCacheCollect(prepared))
		return prepared
	}

	for _, tc := range []struct {
		name     string
		body     string
		validate bool
		stored   bool
	}{
		{name: "a malformed response is not stored when validating", body: `{"data":{"employee":"oops"}}`, validate: true},
		{name: "a well formed response is stored when validating", body: `{"data":{"employee":{"id":1}}}`, validate: true, stored: true},
		{name: "a malformed response is stored when not validating", body: `{"data":{"employee":"oops"}}`, stored: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prepared := collect(t, tc.body, tc.validate)
			require.Equal(t, tc.stored, len(prepared.responseCacheItems) == 1)
			if !tc.stored {
				require.Equal(t, caching.StoreDecisionInvalidResponse, prepared.res.responseCache.StoreDecision)
			}
		})
	}
}
