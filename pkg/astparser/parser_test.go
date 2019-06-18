package astparser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"testing"
)

func TestParser_Parse(t *testing.T) {

	type check func(in *input.Input, doc *ast.Document)

	run := func(inputString string, wantErr bool, checks ...check) {

		in := &input.Input{}
		in.ResetInputBytes([]byte(inputString))
		lexer := &lexer.Lexer{}
		lexer.SetInput(in)
		doc := &ast.Document{}

		parser := NewParser(lexer)

		err := parser.Parse(in, doc)

		if wantErr && err == nil {
			panic("want err, got nil")
		} else if !wantErr && err != nil {
			panic(fmt.Errorf("want nil, got err: %s", err.Error()))
		}

		for _, check := range checks {
			check(in, doc)
		}
	}

	t.Run("no err on empty input", func(t *testing.T) {
		run("", false)
	})
	t.Run("schema", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`schema {
						query: Query
						mutation: Mutation
						subscription: Subscription 
					}`,
				false,
				func(in *input.Input, doc *ast.Document) {
					definition := doc.Definitions[0]
					if definition.Ref != 0 {
						panic("want 0")
					}
					if definition.Kind != ast.SchemaDefinitionKind {
						panic("want SchemaDefinitionKind")
					}
					schema := doc.SchemaDefinitions[0]
					if !schema.RootOperationTypeDefinitions.Next(doc) {
						panic("want next")
					}
					query, queryRef := schema.RootOperationTypeDefinitions.Value()
					if !schema.RootOperationTypeDefinitions.Next(doc) {
						panic("want next")
					}
					if queryRef != 0 {
						panic("want 0")
					}
					if query.OperationType != ast.OperationTypeQuery {
						panic("want OperationTypeQuery")
					}
					name := in.ByteSliceString(query.NamedType.Name)
					if name != "Query" {
						panic(fmt.Errorf("want 'Query', got '%s'", name))
					}
					mutation, mutationRef := schema.RootOperationTypeDefinitions.Value()
					if !schema.RootOperationTypeDefinitions.Next(doc) {
						panic("want next")
					}
					if mutationRef != 1 {
						panic("want 1")
					}
					if mutation.OperationType != ast.OperationTypeMutation {
						panic("want OperationTypeMutation")
					}
					name = in.ByteSliceString(mutation.NamedType.Name)
					if name != "Mutation" {
						panic(fmt.Errorf("want 'Mutation', got '%s'", name))
					}
					subscription, subscriptionRef := schema.RootOperationTypeDefinitions.Value()
					if subscriptionRef != 2 {
						panic("want 2")
					}
					if subscription.OperationType != ast.OperationTypeSubscription {
						panic("want OperationTypeSubscription")
					}
					name = in.ByteSliceString(subscription.NamedType.Name)
					if name != "Subscription" {
						panic(fmt.Errorf("want 'Subscription', got '%s'", name))
					}
				})
		})
		t.Run("with directives", func(t *testing.T) {
			run(`schema @foo @bar(baz: "bal") {
						query: Query 
					}`, false, func(in *input.Input, doc *ast.Document) {
				schema := doc.SchemaDefinitions[0]
				if !schema.Directives.Next(doc) {
					panic("want Next")
				}
				foo, fooRef := schema.Directives.Value()
				if fooRef != 0 {
					panic("want 0")
				}
				fooName := in.ByteSliceString(foo.Name)
				if fooName != "foo" {
					panic("want foo, got: " + fooName)
				}
				if foo.ArgumentList.HasNext() {
					panic("should not HasNext")
				}
				if !schema.Directives.Next(doc) {
					panic("want Next")
				}
				bar, barRef := schema.Directives.Value()
				if barRef != 1 {
					panic("want 1")
				}
				barName := in.ByteSliceString(bar.Name)
				if barName != "bar" {
					panic("want bar, got: " + barName)
				}
				if !bar.ArgumentList.Next(doc) {
					panic("want next")
				}
				baz, bazRef := bar.ArgumentList.Value()
				if bazRef != 0 {
					panic("want 0")
				}
				bazName := in.ByteSliceString(baz.Name)
				if bazName != "baz" {
					panic("want baz, got: " + bazName)
				}
				if baz.Value.Kind != ast.ValueKindString {
					panic("want ValueKindString")
				}
				bal := in.ByteSliceString(baz.Value.Raw)
				if bal != "bal" {
					panic("want bal, got: " + bal)
				}
			})
		})
		t.Run("invalid body missing", func(t *testing.T) {
			run(`schema`, true)
		})
		t.Run("invalid body unclosed", func(t *testing.T) {
			run(`schema {`, true)
		})
		t.Run("invalid directive arguments unclosed", func(t *testing.T) {
			run(`schema @foo( {}`, true)
		})
		t.Run("invalid directive without @", func(t *testing.T) {
			run(`schema foo {}`, true)
		})
	})
	t.Run("object type definition", func(t *testing.T) {
		run(`type Person {
							name: String
						}`, false, func(in *input.Input, doc *ast.Document) {
			person := doc.ObjectTypeDefinitions[0]
			personName := in.ByteSliceString(person.Name)
			if personName != "Person" {
				panic("want person")
			}
			if !person.FieldsDefinition.Next(doc) {
				panic("want next")
			}
			nameString, nameStringRef := person.FieldsDefinition.Value()
			if nameStringRef != 0 {
				panic("want 0")
			}
			name := in.ByteSliceString(nameString.Name)
			if name != "name" {
				panic("want name")
			}
			if nameString.Type.TypeKind != ast.TypeKindNamed {
				panic("want TypeKindNamed")
			}
			if nameString.Type.Reference != 0 {
				panic("want 0")
			}
			stringType := doc.NamedTypes[0]
			stringName := in.ByteSliceString(stringType.Name)
			if stringName != "String" {
				panic("want String")
			}
		})
	})
}

func BenchmarkParse(b *testing.B) {

	inputBytes := []byte(`schema @foo @bar(baz: "bal") {
						query: Query
						mutation: Mutation
						subscription: Subscription 
					}`)

	in := &input.Input{}
	doc := ast.NewDocument()
	parser := NewParser(&lexer.Lexer{})

	var err error

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		in.ResetInputBytes(inputBytes)
		doc.Reset()
		err = parser.Parse(in, doc)
		if err != nil {
			b.Fatal(err)
		}
	}
}
