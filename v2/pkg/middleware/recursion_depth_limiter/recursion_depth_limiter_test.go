package recursion_depth_limiter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestRecursionDepthLimiter(t *testing.T) {
	t.Run("no recursion", func(t *testing.T) {
		run(t, testDefinition, `{
	book(id: 1) {
		id
		author {
			id
			... on Author {
				works {
					publisher {
						supervisor {
							manager {
 								# no matter how deep we go and access employee type, recursion is not being detected
									id
								}
							}
						}
					}
				}
		}
		supervisor {
			id
		}
	}
}`,
			1,
			"",
		)
	})
	t.Run("direct recursion", func(t *testing.T) {
		run(t, testDefinition, `{
	employee(id: 1) {
		id
		manager {
			manager {
				id
			}
		}
	}
}`,
			1,
			"external: Recursion detected: type 'Employee' exceeds allowed depth of 1, locations: [], path: [query,employee,manager,manager]",
		)
	})
	t.Run("indirect recursion", func(t *testing.T) {
		run(t, testDefinition, `{
	employee(id: 1) {
		id
		manager {
			supervisor {
				... on Supervisor {
					subordinates {
						id
						manager1: manager {
							id
						}
					}
				}
			}
		}
	}
}`,
			1,
			"external: Recursion detected: type 'Employee' exceeds allowed depth of 1, locations: [], path: [query,employee,manager,supervisor,$0Supervisor,subordinates,manager1]",
		)
	})
	t.Run("indirect recursion and direct recursion", func(t *testing.T) {
		run(t, testDefinition, `{
	employee(id: 1) {
		id
		manager {
			supervisor {
				... on Supervisor {
					subordinates {
						id
						manager {
							id
							manager { id }
						}
					}
				}
			}
		}
	}
}`,
			2,
			"external: Recursion detected: type 'Employee' exceeds allowed depth of 2, locations: [], path: [query,employee,manager,supervisor,$0Supervisor,subordinates,manager,manager]",
		)
	})
	t.Run("indirect recursion and indirect recursion", func(t *testing.T) {
		run(t, testDefinition, `{
	employee(id: 1) {
		id
		manager {
			supervisor {
				... on Supervisor {
					subordinates {
						id
						manager {
							id
							supervisor { id manager { id } }
						}
					}
				}
			}
		}
	}
}`,
			2,
			"external: Recursion detected: type 'Employee' exceeds allowed depth of 2, locations: [], path: [query,employee,manager,supervisor,$0Supervisor,subordinates,manager,supervisor,manager]",
		)
	})

	t.Run("indirect recursion and indirect recursion book", func(t *testing.T) {
		run(t, testDefinition, `{
	book(id: 1) {
		id
		author {
			id
			... on Author {
				works {
					author { id }
					}
				}
		}
		supervisor {
			id
		}
	}
}`,
			1,
			"external: Recursion detected: type 'Employee' exceeds allowed depth of 1, locations: [], path: [query,book,author,$0Author,works,author]",
		)
	})
}

func run(t *testing.T, definition, operation string, maxRecursionDepth int, expectedErr string) {
	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)

	report := operationreport.Report{}

	astnormalization.NormalizeOperation(&op, &def, &report)
	if report.HasErrors() {
		require.NoError(t, report)
	}

	err := LimitRecursionDepth(&def, &op, maxRecursionDepth)

	if expectedErr != "" {
		assert.EqualError(t, err, expectedErr)
	} else {
		assert.NoError(t, err)
	}
}

const testDefinition = `
schema {
	query: Query
}

scalar ID

type Query {
	employee(id: ID!): Employee
	book(id: ID!): Book  
}

interface Employee {
	id: ID!
	supervisor: Employee
	manager: Employee
}

type Author implements Employee {
	id: ID!
	works: [Book]
	supervisor: Employee
	manager: Employee
}

type Manager implements Employee {
	id: ID!
	subordinates: [Employee]
	supervisor: Employee
	manager: Employee
}

type Supervisor implements Employee {
	id: ID!
	subordinates: [Employee]
	supervisor: Employee
	manager: Employee
}

type Book {
	id: ID!, 
	author: Employee
	publisher: Employee
}
`
