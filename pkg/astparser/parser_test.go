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
	type action func(parser *Parser) error

	parse := func() action {
		return func(parser *Parser) error {
			return parser.Parse(parser.input, parser.document)
		}
	}

	parseType := func() action {
		return func(parser *Parser) error {
			parser.parseType()
			return parser.err
		}
	}

	run := func(inputString string, action func() action, wantErr bool, checks ...check) {

		in := &input.Input{}
		in.ResetInputBytes([]byte(inputString))
		lex := &lexer.Lexer{}
		lex.SetInput(in)
		doc := &ast.Document{}

		parser := NewParser(lex)
		parser.input = in
		parser.document = doc

		err := action()(parser)

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
		run("", parse, false)
	})
	t.Run("schema", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`schema {
						query: Query
						mutation: Mutation
						subscription: Subscription 
					}`, parse,
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
					}`, parse, false, func(in *input.Input, doc *ast.Document) {
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
			run(`schema`, parse, true)
		})
		t.Run("invalid body unclosed", func(t *testing.T) {
			run(`schema {`, parse, true)
		})
		t.Run("invalid directive arguments unclosed", func(t *testing.T) {
			run(`schema @foo( {}`, parse, true)
		})
		t.Run("invalid directive without @", func(t *testing.T) {
			run(`schema foo {}`, parse, true)
		})
	})
	t.Run("object type definition", func(t *testing.T) {
		t.Run("complex", func(t *testing.T) {
			run(`type Person implements Foo & Bar {
							name: String
							"age of the person"
							age: Int
							"""
							date of birth
							"""
							dateOfBirth: Date
						}`, parse, false, func(in *input.Input, doc *ast.Document) {
				person := doc.ObjectTypeDefinitions[0]
				personName := in.ByteSliceString(person.Name)
				if personName != "Person" {
					panic("want person")
				}

				// interfaces

				if !person.ImplementsInterfaces.Next(doc) {
					panic("want next")
				}
				implementsFoo, implementsFooRef := person.ImplementsInterfaces.Value()
				if implementsFooRef != 0 {
					panic("want 0")
				}
				if implementsFoo.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(implementsFoo.Name) != "Foo" {
					panic("want Foo")
				}

				if !person.ImplementsInterfaces.Next(doc) {
					panic("want next")
				}
				implementsBar, implementsBarRef := person.ImplementsInterfaces.Value()
				if implementsBarRef != 1 {
					panic("want 1")
				}
				if implementsBar.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(implementsBar.Name) != "Bar" {
					panic("want Bar")
				}

				// field definitions
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
				nameStringType := doc.Types[nameString.Type]
				if nameStringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				stringName := in.ByteSliceString(nameStringType.Name)
				if stringName != "String" {
					panic("want String")
				}
				if !person.FieldsDefinition.Next(doc) {
					panic("want netxt")
				}
				ageField, ageFieldRef := person.FieldsDefinition.Value()
				if ageFieldRef != 1 {
					panic("want 1")
				}
				if !ageField.Description.IsDefined {
					panic("want true")
				}
				if ageField.Description.IsBlockString {
					panic("want false	")
				}
				if in.ByteSliceString(ageField.Description.Body) != "age of the person" {
					panic("want 'age of the person'")
				}
				if in.ByteSliceString(ageField.Name) != "age" {
					panic("want age")
				}
				intType := doc.Types[ageField.Type]
				if intType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(intType.Name) != "Int" {
					panic("want Int")
				}
				if !person.FieldsDefinition.Next(doc) {
					panic("want next")
				}
				dateOfBirthField, dateOfBirthFieldRef := person.FieldsDefinition.Value()
				if dateOfBirthFieldRef != 2 {
					panic("want 2")
				}
				if in.ByteSliceString(dateOfBirthField.Name) != "dateOfBirth" {
					panic("want dateOfBirth")
				}
				if !dateOfBirthField.Description.IsDefined {
					panic("want true")
				}
				if !dateOfBirthField.Description.IsBlockString {
					panic("want true")
				}
				if in.ByteSliceString(dateOfBirthField.Description.Body) != `
							date of birth
							` {
					panic(fmt.Sprintf("want 'date of birth' got: '%s'", in.ByteSliceString(dateOfBirthField.Description.Body)))
				}
				dateType := doc.Types[dateOfBirthField.Type]
				if in.ByteSliceString(dateType.Name) != "Date" {
					panic("want Date")
				}
			})
		})
		t.Run("implements optional variant", func(t *testing.T) {
			run(`type Person implements & Foo & Bar {}`, parse, false, func(in *input.Input, doc *ast.Document) {
				person := doc.ObjectTypeDefinitions[0]
				personName := in.ByteSliceString(person.Name)
				if personName != "Person" {
					panic("want person")
				}
				// interfaces

				if !person.ImplementsInterfaces.Next(doc) {
					panic("want next")
				}
				implementsFoo, implementsFooRef := person.ImplementsInterfaces.Value()
				if implementsFooRef != 0 {
					panic("want 0")
				}
				if implementsFoo.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(implementsFoo.Name) != "Foo" {
					panic("want Foo")
				}

				if !person.ImplementsInterfaces.Next(doc) {
					panic("want next")
				}
				implementsBar, implementsBarRef := person.ImplementsInterfaces.Value()
				if implementsBarRef != 1 {
					panic("want 1")
				}
				if implementsBar.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(implementsBar.Name) != "Bar" {
					panic("want Bar")
				}
			})
		})
		t.Run("implements & without next", func(t *testing.T) {
			run(`type Person implements Foo & {}`, parse, true)
		})
	})
	t.Run("type", func(t *testing.T) {
		t.Run("named", func(t *testing.T) {
			run("String", parseType, false, func(in *input.Input, doc *ast.Document) {
				stringType := doc.Types[0]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null named", func(t *testing.T) {
			run("String!", parseType, false, func(in *input.Input, doc *ast.Document) {
				nonNull := doc.Types[1]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				stringType := doc.Types[nonNull.OfType]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of named", func(t *testing.T) {
			run("[String]!", parseType, false, func(in *input.Input, doc *ast.Document) {
				nonNull := doc.Types[2]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				list := doc.Types[nonNull.OfType]
				if list.TypeKind != ast.TypeKindList {
					panic("want TypeKindList")
				}
				stringType := doc.Types[list.OfType]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of non null named", func(t *testing.T) {
			run("[String!]!", parseType, false, func(in *input.Input, doc *ast.Document) {
				nonNull := doc.Types[3]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				list := doc.Types[nonNull.OfType]
				if list.TypeKind != ast.TypeKindList {
					panic("want TypeKindList")
				}
				nonNull = doc.Types[list.OfType]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				stringType := doc.Types[nonNull.OfType]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of non null list of named", func(t *testing.T) {
			run("[[String]!]!", parseType, false, func(in *input.Input, doc *ast.Document) {
				nonNull := doc.Types[4]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				list := doc.Types[nonNull.OfType]
				if list.TypeKind != ast.TypeKindList {
					panic("want TypeKindList")
				}
				nonNull = doc.Types[list.OfType]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				list = doc.Types[nonNull.OfType]
				if list.TypeKind != ast.TypeKindList {
					panic("want TypeKindList")
				}
				stringType := doc.Types[list.OfType]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of non null list of non null named", func(t *testing.T) {
			run("[[String!]!]!", parseType, false, func(in *input.Input, doc *ast.Document) {
				nonNull := doc.Types[5]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				list := doc.Types[nonNull.OfType]
				if list.TypeKind != ast.TypeKindList {
					panic("want TypeKindList")
				}
				nonNull = doc.Types[list.OfType]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				list = doc.Types[nonNull.OfType]
				if list.TypeKind != ast.TypeKindList {
					panic("want TypeKindList")
				}
				nonNull = doc.Types[list.OfType]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				stringType := doc.Types[nonNull.OfType]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if in.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("err unexpected bang", func(t *testing.T) {
			run("!", parseType, true)
		})
		t.Run("err empty list", func(t *testing.T) {
			run("[]", parseType, true)
		})
		t.Run("err incomplete list", func(t *testing.T) {
			run("[", parseType, true)
		})
		t.Run("err unclosed list", func(t *testing.T) {
			run("[String", parseType, true)
		})
		t.Run("err unclosed list with bang", func(t *testing.T) {
			run("[String!", parseType, true)
		})
		t.Run("err double bang", func(t *testing.T) {
			run("String!!", parseType, true)
		})
		t.Run("err list close at beginning", func(t *testing.T) {
			run("]String", parseType, true)
		})
	})
}

func BenchmarkParse(b *testing.B) {

	inputBytes := []byte(`	schema @foo @bar(baz: "bal") {
								query: Query
								mutation: Mutation
								subscription: Subscription 
							}

							type Person implements Foo & Bar {
								name: String
								"age of the person"
								age: Int
								"""
								date of birth
								"""
								dateOfBirth: Date
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
