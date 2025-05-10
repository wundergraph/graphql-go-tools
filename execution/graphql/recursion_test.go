package graphql

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

/*──────── helper ────────*/

func runRecursion(
	t *testing.T,
	sdl, query string,
	maxDepth int,
	wantErr bool,
) {
	schema, _ := astparser.ParseGraphqlDocumentString(sdl)
	op, _ := astparser.ParseGraphqlDocumentString(query)

	// normalise once so that spreads / inline fragments are flattened
	normReport := operationreport.Report{}
	astnormalization.NormalizeOperation(&op, &schema, &normReport)
	require.False(t, normReport.HasErrors(), "invalid test documents: %v", normReport)

	calc := NewRecursionCalculator(maxDepth)
	res, err := calc.Calculate(&op, &schema)
	require.NoError(t, err)

	if wantErr {
		require.NotEmpty(t, res.Errors, "expected recursion error but got none")
	} else {
		require.Empty(t, res.Errors, "unexpected recursion error: %v", res.Errors)
	}
}

/*──────── tests ─────────*/

func TestRecursionCalculator(t *testing.T) {
	const scalars = "scalar ID\nscalar String\n"

	employeeSDL := scalars + `
type Query   { employee(id: ID!): Employee }
type Employee{ id: ID manager: Employee }
schema { query: Query }`

	bookSDL := scalars + `
type Query  { book(id: ID!): Book }
type Book   { id: ID author: Author }
type Author { id: ID works: [Book] }
schema { query: Query }`

	// direct chain depth-3
	direct := `
{
  employee(id:"1"){
    manager{
      manager{ id }
    }
  }
}`

	// indirect Book–Author loop depth-3
	indirect := `
{
  book(id:"1"){
    author{
      works{
        author{
          works{ id }
        }
      }
    }
  }
}`

	t.Run("scalar only – ok", func(t *testing.T) {
		runRecursion(t, employeeSDL, `{ employee(id:"1"){ id } }`, 1, false)
	})

	t.Run("direct recursion over limit – err", func(t *testing.T) {
		runRecursion(t, employeeSDL, direct, 1, true)
	})

	t.Run("direct recursion within limit – ok", func(t *testing.T) {
		runRecursion(t, employeeSDL, direct, 3, false)
	})

	t.Run("indirect recursion within limit – ok", func(t *testing.T) {
		runRecursion(t, bookSDL, indirect, 3, false)
	})

	t.Run("indirect recursion over limit – err", func(t *testing.T) {
		runRecursion(t, bookSDL, indirect, 2, true)
	})
}
