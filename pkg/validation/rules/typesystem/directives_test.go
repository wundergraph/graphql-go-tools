package typesystem

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
	"testing"
)

func TestValidateTypeSystemDefinition_Directives(t *testing.T) {
	run := func(input string, rule rules.Rule, valid bool) {
		p := parser.NewParser()
		err := p.ParseTypeSystemDefinition([]byte(input))
		if err != nil {
			panic(err)
		}

		l := lookup.New(p, 256)

		walker := lookup.NewWalker(1024, 8)
		walker.SetLookup(l)
		walker.WalkTypeSystemDefinition()

		result := rule(l, walker)

		if valid != result.Valid {
			panic(fmt.Errorf("want valid: %t, got: %t (result: %+v, subName: %s)", valid, result.Valid, result, l.CachedName(result.Meta.SubjectNameRef)))
		}
	}

	t.Run("directives are defined", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			run(`	directive @addArgumentFromContext(
						name: String!
						contextKey: String!
					) on FIELD_DEFINITION
					type Query {
						documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
					}`,
				DirectivesAreDefined(), true)
		})
		t.Run("invalid", func(t *testing.T) {
			run(`	type Query {
							documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
						}`,
				DirectivesAreDefined(), false)
		})
	})
	t.Run("directives are in valid locations", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			run(`	directive @addArgumentFromContext(
							name: String!
							contextKey: String!
						) on FIELD_DEFINITION
						type Query {
							documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
						}`,
				DirectivesAreInValidLocations(), true)
		})
		t.Run("invalid", func(t *testing.T) {
			run(`	directive @addArgumentFromContext(
							name: String!
							contextKey: String!
						) on FIELD_DEFINITION
						type Query @addArgumentFromContext(name: "user",contextKey: "user") {
							documents: [Document]
						}`,
				DirectivesAreInValidLocations(), false)
		})
	})
	t.Run("directives are unique per location", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			run(`	type Query {
							documents: [Document] @foo
						}`,
				DirectivesAreUniquePerLocation(), true)
		})
		t.Run("invalid", func(t *testing.T) {
			run(`	type Query {
							documents: [Document] @foo @foo
						}`,
				DirectivesAreUniquePerLocation(), false)
		})
		t.Run("invalid", func(t *testing.T) {
			run(`	type Query @foo @foo {
							documents: [Document]
						}`,
				DirectivesAreUniquePerLocation(), false)
		})
		t.Run("valid", func(t *testing.T) {
			run(`	type Query {
							documents: [Document] @foo @bar
						}`,
				DirectivesAreUniquePerLocation(), true)
		})
		t.Run("valid", func(t *testing.T) {
			run(`	type Query @foo {
							documents: [Document] @foo
						}`,
				DirectivesAreUniquePerLocation(), true)
		})
	})
	t.Run("directives have required arguments", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			run(`	directive @addArgumentFromContext(
							name: String!
							contextKey: String!
						) on FIELD_DEFINITION
						type Query {
							documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
						}`,
				DirectivesHaveRequiredArguments(), true)
		})
		t.Run("arg missing", func(t *testing.T) {
			run(`	directive @addArgumentFromContext(
							name: String!
							contextKey: String!
						) on FIELD_DEFINITION
						type Query {
							documents: [Document] @addArgumentFromContext(name: "user")
						}`,
				DirectivesHaveRequiredArguments(), false)
		})
		t.Run("optional second arg ok", func(t *testing.T) {
			run(`	directive @addArgumentFromContext(
							name: String!
							contextKey: String
						) on FIELD_DEFINITION
						type Query {
							documents: [Document] @addArgumentFromContext(name: "user")
						}`,
				DirectivesHaveRequiredArguments(), true)
		})
		t.Run("wrong arg type", func(t *testing.T) {
			run(`	directive @addArgumentFromContext(
							name: String!
							contextKey: String!
						) on FIELD_DEFINITION
						type Query {
							documents: [Document] @addArgumentFromContext(name: 123, contextKey: "user")
						}`,
				DirectivesHaveRequiredArguments(), false)
		})
	})
}
