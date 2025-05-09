package recursion_guard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func runLong(
	t *testing.T,
	sdl, op string,
	maxDepth int,
	wantErr bool,
) {
	schema := unsafeparser.ParseGraphqlDocumentString(sdl)
	doc := unsafeparser.ParseGraphqlDocumentString(op)
	rep := operationreport.Report{}
	astnormalization.NormalizeOperation(&doc, &schema, &rep)
	require.False(t, rep.HasErrors(), "schema/op invalid: %v", rep)

	guard := NewRecursionGuard(maxDepth)
	guard.Do(&doc, &schema, &rep)

	if wantErr {
		require.True(t, rep.HasErrors(), "expected recursion error")
	} else {
		require.False(t, rep.HasErrors(), "unexpected error: %v", rep.ExternalErrors)
	}
}

func TestRecursionGuard_DeepPaths(t *testing.T) {
	const scalars = "scalar ID\nscalar String\n"

	/*──── 1. direct self‑recursion chain ────*/
	const employeeSDL = scalars + `
type Query   { employee(id: ID!): Employee }
type Employee{ id: ID manager: Employee }
schema { query: Query }
`
	chain := strings.Repeat("manager{", 9) + "id" + strings.Repeat("}", 9)
	longEmployeeQuery := `
{
  employee(id:"1"){
    ` + chain + `
  }
}`
	t.Run("Employee depth‑10 vs limit‑3 -> ERR", func(t *testing.T) {
		runLong(t, employeeSDL, longEmployeeQuery, 3, true)
	})
	t.Run("Employee depth‑10 vs limit‑10 -> OK", func(t *testing.T) {
		runLong(t, employeeSDL, longEmployeeQuery, 10, false)
	})

	const bookSDL = scalars + `
type Query  { book(id: ID!): Book }
type Book   { id: ID author: Author }
type Author { id: ID works: [Book] }
schema { query: Query }
`

	indirect := ""
	for i := 0; i < 5; i++ {
		indirect += "author{works{"
	}
	indirect += "id"
	for i := 0; i < 5; i++ {
		indirect += "}}"
	}
	longBookQuery := `
{
  book(id:"42"){
    ` + indirect + `
  }
}`
	t.Run("Book depth‑6 vs limit‑2 -> ERR", func(t *testing.T) {
		runLong(t, bookSDL, longBookQuery, 2, true)
	})
	t.Run("Book depth‑6 vs limit‑6 -> OK", func(t *testing.T) {
		runLong(t, bookSDL, longBookQuery, 6, false)
	})

	mixedQuery := `
{
  employee(id:"1"){             # root Employee
    id
    manager{                    # Employee depth 2
      id
      peer1: manager{ id }      # Employee depth 3 (alias branch)
      peer2: manager{           # Employee depth 3 again
        manager{                # Employee depth 4 -> should overflow @ limit 3
          id
        }
      }
    }
  }
}`
	t.Run("mixed branch, limit‑3 -> ERR", func(t *testing.T) {
		runLong(t, employeeSDL, mixedQuery, 3, true)
	})
	t.Run("mixed branch, limit‑5 -> OK", func(t *testing.T) {
		runLong(t, employeeSDL, mixedQuery, 5, false)
	})
}
