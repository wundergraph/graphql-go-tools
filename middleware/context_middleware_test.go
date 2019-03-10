package middleware

import (
	"github.com/jensneuse/graphql-go-tools/pkg/testhelper"
	"testing"
)

func TestContextMiddleware(t *testing.T) {

	got := InvokeMiddleware(&ContextMiddleware{}, publicSchema, publicQuery)
	want := testhelper.UglifyRequestString(privateQuery)

	if want != got {
		t.Errorf("\nwant:\n%s\ngot:\n%s", want, got)
	}
}

/*func TestContextMiddleware(t *testing.T) {
	es := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// set context that would usually be set in other application middleware
		userCtx := context.WithValue(r.Context(), "user", "jsmith@example.org")
		r = r.WithContext(userCtx)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != privateQuery {
			t.Errorf("Expected %s, got %s", privateQuery, body)
		}
	}))
	defer es.Close()

	schemaProvider := handler.NewStaticSchemaProvider([]byte(publicSchema))

	ph := handler.NewHttpProxyHandler(es.URL, schemaProvider, &ContextMiddleware{})
	ts := httptest.NewServer(ph)
	defer ts.Close()

	t.Run("Test context middleware", func(t *testing.T) {
		r, err := http.NewRequest("POST", ts.URL, strings.NewReader(publicQuery))
		ctx := context.WithValue(context.Background(), "user", "jsmith@example.org")
		r = r.WithContext(ctx)
		if err != nil {
			t.Error(err)
		}
		client := http.DefaultClient
		_, err = client.Do(r)
		if err != nil {
			t.Error(err)
		}
	})
}*/

const publicSchema = `
directive @addArgumentFromContext(
	name: String!
	contextKey: String!
) on FIELD_DEFINITION

scalar String

schema {
	query: Query
}

type Query {
	documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
}
`

// This schema is unused, left for reference
const _ = `
schema {
	query: Query
}

type Query {
	documents(user: String!): [Document]
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
}
`

const publicQuery = `
query myDocuments {
	documents {
		sensitiveInformation
	}
}
`

const privateQuery = `
query myDocuments {
	documents(user: "jsmith@example.org") {
		sensitiveInformation
	}
}
`
