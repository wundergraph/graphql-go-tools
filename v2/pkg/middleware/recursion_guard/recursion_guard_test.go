// recursion_guard_test.go
package recursion_guard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

/* ------------------------------------------------------------------ */
/* helper                                                             */
/* ------------------------------------------------------------------ */

func runGuard(
	t *testing.T,
	sdl, query string,
	maxDepth int,
	wantErr bool,
	wantSubstr string,
) {
	schema := unsafeparser.ParseGraphqlDocumentString(sdl)
	op := unsafeparser.ParseGraphqlDocumentString(query)
	report := operationreport.Report{}

	astnormalization.NormalizeOperation(&op, &schema, &report)
	require.False(t, report.HasErrors(), "schema / op must be valid: %v", report)

	guard := NewRecursionGuard(maxDepth)
	guard.Do(&op, &schema, &report)

	if wantErr {
		require.True(t, report.HasErrors(), "expected recursion error")
		if wantSubstr != "" {
			assert.Contains(t, report.ExternalErrors[0].Message, wantSubstr)
		}
	} else {
		require.False(t, report.HasErrors(), "unexpected recursion error: %v", report.ExternalErrors)
	}
}

/* ------------------------------------------------------------------ */
/* tests                                                              */
/* ------------------------------------------------------------------ */

func TestRecursionGuard(t *testing.T) {
	t.Run("scalar only – no recursion", func(t *testing.T) {
		runGuard(t, employeeSDL, `{ employee(id:"1"){ id } }`, 1, false, "")
	})

	t.Run("direct recursion over limit", func(t *testing.T) {
		runGuard(t, employeeSDL, `
			{
			  employee(id:"1"){
			    manager{
			      manager{ id }
			    }
			  }
			}`, 1, true, "employee.manager.manager")
	})

	t.Run("indirect recursion within limit", func(t *testing.T) {
		runGuard(t, bookSDL, `
			{
			  book(id:"1"){
			    author{
			      works{
			        author{ id }
			      }
			    }
			  }
			}`, 2, false, "")
	})

	t.Run("indirect recursion over limit", func(t *testing.T) {
		runGuard(t, bookSDL, `
			{
			  book(id:"1"){
			    author{
			      works{
			        author{
			          works{ id }  # third Book
			        }
			      }
			    }
			  }
			}`, 2, true, "works.author.works")
	})
}

/* ------------------------------------------------------------------ */
/* minimal SDLs with built-in scalars declared                        */
/* ------------------------------------------------------------------ */

const employeeSDL = `
scalar ID
scalar String

type Query   { employee(id: ID!): Employee }
type Employee{ id: ID manager: Employee }
schema { query: Query }
`

const bookSDL = `
scalar ID
scalar String

type Query  { book(id: ID!): Book }
type Book   { id: ID author: Author }
type Author { id: ID works: [Book] }
schema { query: Query }
`
