package recursionguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func TestRecursionGuard(t *testing.T) {
	run := func(t *testing.T, schemaSDL, query string, maxDepth int, wantErr bool, wantMsgSubstr string) {
		schema := unsafeparser.ParseGraphqlDocumentString(schemaSDL)
		op := unsafeparser.ParseGraphqlDocumentString(query)
		report := operationreport.Report{}

		astnormalization.NormalizeOperation(&op, &schema, &report)

		guard := NewRecursionGuard(maxDepth)
		guard.Do(&op, &schema, &report)

		if wantErr {
			require.True(t, report.HasErrors(), "expected error")
			assert.Contains(t, report.ExternalErrors[0].Message, wantMsgSubstr)
		} else {
			require.False(t, report.HasErrors(), "unexpected error: %v", report.InternalErrors)
		}
	}

	t.Run("no recursion (scalar only)", func(t *testing.T) {
		run(t, employeeSDL, `{ employee(id:"1"){ id } }`, 1, false, "")
	})

	t.Run("direct recursion over limit", func(t *testing.T) {
		run(t, employeeSDL, `
			{
			  employee(id:"1"){
			    manager{
			      manager{ id }
			    }
			  }
			}`, 1, true, "employee.manager.manager")
	})

	t.Run("direct recursion within limit", func(t *testing.T) {
		run(t, employeeSDL, `
			{
			  employee(id:"1"){
			    manager{ id }
			  }
			}`, 1, false, "")
	})

	t.Run("indirect recursion within limit", func(t *testing.T) {
		run(t, bookSDL, `
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
		run(t, bookSDL, `
			{
				book(id:"1"){
					author{
						works{
							author{
								works{ id }   # third Book on same path
							}
						}
					}
				}
			}`, 2, true, "book.author.works.author.works")
	})
}

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
