package astparser

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/keyword"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestParser_Parse(t *testing.T) {

	type check func(doc *ast.Document, extra interface{})
	type action func(parser *Parser) (interface{}, operationreport.Report)

	parse := func() action {
		return func(parser *Parser) (interface{}, operationreport.Report) {
			report := operationreport.Report{}
			parser.Parse(parser.document, &report)
			return nil, report
		}
	}

	parseType := func() action {
		return func(parser *Parser) (interface{}, operationreport.Report) {
			report := operationreport.Report{}
			parser.report = &report
			parser.tokenize()
			ref := parser.ParseType()
			return ref, report
		}
	}

	parseValue := func() action {
		return func(parser *Parser) (interface{}, operationreport.Report) {
			report := operationreport.Report{}
			parser.report = &report
			parser.tokenize()
			value := parser.ParseValue()
			return value, report
		}
	}

	parseSelectionSet := func() action {
		return func(parser *Parser) (interface{}, operationreport.Report) {
			report := operationreport.Report{}
			parser.report = &report
			parser.tokenize()
			set, _ := parser.parseSelectionSet()
			return set, report
		}
	}

	parseFragmentSpread := func() action {
		return func(parser *Parser) (interface{}, operationreport.Report) {
			report := operationreport.Report{}
			parser.report = &report
			parser.tokenize()
			fragmentSpread := parser.parseFragmentSpread(position.Position{})
			return parser.document.FragmentSpreads[fragmentSpread], report
		}
	}

	parseInlineFragment := func() action {
		return func(parser *Parser) (interface{}, operationreport.Report) {
			report := operationreport.Report{}
			parser.report = &report
			parser.tokenize()
			inlineFragment := parser.parseInlineFragment(position.Position{})
			return parser.document.InlineFragments[inlineFragment], report
		}
	}

	parseVariableDefinitionList := func() action {
		return func(parser *Parser) (interface{}, operationreport.Report) {
			report := operationreport.Report{}
			parser.report = &report
			parser.tokenize()
			variableDefinitionList := parser.parseVariableDefinitionList()
			return variableDefinitionList, report
		}
	}

	wantIndexedNode := func(key string, kind ast.NodeKind) check {
		return func(doc *ast.Document, extra interface{}) {
			node, exists := doc.Index.FirstNodeByNameStr(key)
			if !exists {
				panic("want true")
			}
			if node.Kind != kind {
				panic(fmt.Errorf("want kind: %s, got: %s", kind, node.Kind))
			}
		}
	}

	run := func(inputString string, action func() action, wantErr bool, checks ...check) {

		doc := ast.NewDocument()
		doc.Input.ResetInputBytes([]byte(inputString))

		parser := NewParser()
		parser.document = doc

		extra, err := action()(parser)

		if wantErr && !err.HasErrors() {
			panic("want report, got nil")
		} else if !wantErr && err.HasErrors() {
			panic(fmt.Errorf("want nil, got report: %s", err.Error()))
		}

		for _, check := range checks {
			check(doc, extra)
		}
	}

	t.Run("tokenize", func(t *testing.T) {

		doc := ast.NewDocument()
		doc.Input.ResetInputBytes([]byte(
			`schema {
				query: Query
				mutation: Mutation
				subscription: Subscription 
			}`,
		))

		parser := NewParser()
		parser.document = doc
		parser.tokenize()

		for i, want := range []keyword.Keyword{
			keyword.IDENT, keyword.LBRACE,
			keyword.IDENT, keyword.COLON, keyword.IDENT,
			keyword.IDENT, keyword.COLON, keyword.IDENT,
			keyword.IDENT, keyword.COLON, keyword.IDENT,
			keyword.RBRACE,
		} {
			parser.peek()
			got := parser.read().Keyword
			if got != want {
				t.Fatalf("want keyword %s @ %d, got: %s", want, i, got)
			}
		}
	})

	t.Run("no report on empty input", func(t *testing.T) {
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
				func(doc *ast.Document, extra interface{}) {
					definition := doc.RootNodes[0]
					if definition.Ref != 0 {
						panic("want 0")
					}
					if definition.Kind != ast.NodeKindSchemaDefinition {
						panic("want NodeKindSchemaDefinition")
					}
					schema := doc.SchemaDefinitions[0]
					query := doc.RootOperationTypeDefinitions[schema.RootOperationTypeDefinitions.Refs[0]]
					if query.OperationType != ast.OperationTypeQuery {
						panic("want OperationTypeQuery")
					}
					name := doc.Input.ByteSliceString(query.NamedType.Name)
					if name != "Query" {
						panic(fmt.Errorf("want 'Query', got '%s'", name))
					}
					mutation := doc.RootOperationTypeDefinitions[schema.RootOperationTypeDefinitions.Refs[1]]
					if mutation.OperationType != ast.OperationTypeMutation {
						panic("want OperationTypeMutation")
					}
					name = doc.Input.ByteSliceString(mutation.NamedType.Name)
					if name != "Mutation" {
						panic(fmt.Errorf("want 'Mutation', got '%s'", name))
					}
					subscription := doc.RootOperationTypeDefinitions[schema.RootOperationTypeDefinitions.Refs[2]]
					if subscription.OperationType != ast.OperationTypeSubscription {
						panic("want OperationTypeSubscription")
					}
					name = doc.Input.ByteSliceString(subscription.NamedType.Name)
					if name != "Subscription" {
						panic(fmt.Errorf("want 'Subscription', got '%s'", name))
					}
				})
		})

		t.Run("with description", func(t *testing.T) {
			run(`
					"this is a schema"
					schema {
						query: Query 
					}`, parse,
				false,
				func(doc *ast.Document, extra interface{}) {
					definition := doc.RootNodes[0]
					if definition.Ref != 0 {
						panic("want 0")
					}
					if definition.Kind != ast.NodeKindSchemaDefinition {
						panic("want NodeKindSchemaDefinition")
					}
					schema := doc.SchemaDefinitions[0]
					if !schema.Description.IsDefined {
						panic("want schema description to be defined")
					}
					description := doc.Input.ByteSliceString(schema.Description.Content)
					if description != "this is a schema" {
						panic(fmt.Errorf("want 'this is a schema', got '%s'", description))
					}
					query := doc.RootOperationTypeDefinitions[schema.RootOperationTypeDefinitions.Refs[0]]
					if query.OperationType != ast.OperationTypeQuery {
						panic("want OperationTypeQuery")
					}
					name := doc.Input.ByteSliceString(query.NamedType.Name)
					if name != "Query" {
						panic(fmt.Errorf("want 'Query', got '%s'", name))
					}
				})
		})

		t.Run("with directives", func(t *testing.T) {
			run(`schema @foo @bar(baz: "bal") {
						query: Query 
					}`, parse, false, func(doc *ast.Document, extra interface{}) {
				schema := doc.SchemaDefinitions[0]
				foo := doc.Directives[schema.Directives.Refs[0]]
				fooName := doc.Input.ByteSliceString(foo.Name)
				if fooName != "foo" {
					panic("want foo, got: " + fooName)
				}
				if len(foo.Arguments.Refs) != 0 {
					panic("should not HasNext")
				}
				bar := doc.Directives[schema.Directives.Refs[1]]
				barName := doc.Input.ByteSliceString(bar.Name)
				if barName != "bar" {
					panic("want bar, got: " + barName)
				}
				baz := doc.Arguments[bar.Arguments.Refs[0]]
				bazName := doc.Input.ByteSliceString(baz.Name)
				if bazName != "baz" {
					panic("want baz, got: " + bazName)
				}
				if baz.Value.Kind != ast.ValueKindString {
					panic("want ValueKindString")
				}
				bal := doc.Input.ByteSliceString(doc.StringValues[baz.Value.Ref].Content)
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
		t.Run("no report on schema with comments everywhere", func(t *testing.T) {
			run(`
				#comment
				scalar #comment
				Date #comment
				
				schema #comment
				{ #comment
				  query#comment
				  :#comment
				  #comment
				  Query#comment
				  #comment
				}#comment
				
				#comment
				type#comment
				Query#comment
				{#comment
				  me#comment
				  :#comment
				  User#comment
				  !#comment
				  user(#comment
					id#comment
					:#comment
					ID#comment
					!#comment
				  )#comment
				  :#comment
				  User#comment
				  allUsers#comment
				  :#comment
				  [#comment
					#comment
					User#comment
				  ]#comment
				  search#comment
				  (#comment
					term#comment
					:#comment
					String#comment
					!#comment
				  )#comment
				  :#comment
				  [#comment
					SearchResult#comment
					!#comment
				  ]#comment
				  !#comment
				  myChats:#comment
				  [#comment
					Chat#comment
					!#comment
				  ]!#comment
				}
				
				enum#comment
				Role#comment
				{#comment
				  #comment
				  USER#comment
				  ,#comment
				  ADMIN#comment
				  ,#comment
				  #comment
				}#comment
				
				interface#comment
				Node {#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				}#comment
				
				union #comment
				SearchResult#comment
				=#comment
				User#comment
				|#comment
				Chat#comment
				|#comment
				ChatMessage#comment
				
				type#comment
				User#comment
				implements#comment
				Node#comment
				{#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				  username#comment
				  :#comment
				  String#comment
				  !#comment
				  email#comment
				  :#comment
				  String#comment
				  !#comment
				  role#comment
				  :#comment
				  Role#comment
				  !#comment
				}#comment
				
				type#comment
				Chat#comment
				implements#comment
				Node#comment
				{#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				  users#comment
				  :#comment
				  [#comment
					User#comment
					!#comment
				  ]!#comment
				  messages#comment
				  :#comment
				  [#comment
					ChatMessage#comment
					!#comment
				  ]#comment
				  !#comment
				  #comment
				}#comment
				
				type#comment
				ChatMessage#comment
				implements#comment
				Node#comment
				{#comment
				  id#comment
				  :#comment
				  ID#comment
				  !#comment
				  content#comment
				  :#comment
				  String#comment
				  !#comment
				  time#comment
				  :#comment
				  Date#comment
				  !#comment
				  user#comment
				  :#comment
				  User#comment
				  !#comment
				  #comment
				}#comment`, parse, false)
		})
	})
	t.Run("schema extension", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`extend schema {
						query: Query
						mutation: Mutation
						subscription: Subscription 
					}`, parse, false,
				func(doc *ast.Document, extra interface{}) {

					schema := doc.SchemaExtensions[0]

					query := doc.RootOperationTypeDefinitions[schema.RootOperationTypeDefinitions.Refs[0]]
					if query.OperationType != ast.OperationTypeQuery {
						panic("want OperationTypeQuery")
					}
					name := doc.Input.ByteSliceString(query.NamedType.Name)
					if name != "Query" {
						panic(fmt.Errorf("want 'Query', got '%s'", name))
					}

					mutation := doc.RootOperationTypeDefinitions[schema.RootOperationTypeDefinitions.Refs[1]]
					if mutation.OperationType != ast.OperationTypeMutation {
						panic("want OperationTypeMutation")
					}
					name = doc.Input.ByteSliceString(mutation.NamedType.Name)
					if name != "Mutation" {
						panic(fmt.Errorf("want 'Mutation', got '%s'", name))
					}
					subscription := doc.RootOperationTypeDefinitions[schema.RootOperationTypeDefinitions.Refs[2]]
					if subscription.OperationType != ast.OperationTypeSubscription {
						panic("want OperationTypeSubscription")
					}
					name = doc.Input.ByteSliceString(subscription.NamedType.Name)
					if name != "Subscription" {
						panic(fmt.Errorf("want 'Subscription', got '%s'", name))
					}
				})
		})

		t.Run("empty with directive", func(t *testing.T) {
			run(`extend schema @myDirective`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					require.Len(t, doc.SchemaExtensions, 1)

					schema := doc.SchemaExtensions[0]
					require.Len(t, schema.RootOperationTypeDefinitions.Refs, 0)
					require.True(t, schema.HasDirectives)

					directive := doc.Directives[schema.Directives.Refs[0]]
					assert.Equal(t, "myDirective", doc.Input.ByteSliceString(directive.Name))
				})
		})

		t.Run("empty", func(t *testing.T) {
			run(`extend schema`, parse, true)
		})
	})
	t.Run("object type extension", func(t *testing.T) {
		t.Run("complex", func(t *testing.T) {
			run(`extend type Person implements Foo & Bar {
							name: String
							"age of the person"
							age: Int
							"""
							date of birth
							"""
							dateOfBirth: Date
						}`, parse, false, func(doc *ast.Document, extra interface{}) {

				person := doc.ObjectTypeExtensions[0]
				personName := doc.Input.ByteSliceString(person.Name)
				if personName != "Person" {
					panic("want person")
				}

				// interfaces

				implementsFoo := doc.Types[person.ImplementsInterfaces.Refs[0]]
				if implementsFoo.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(implementsFoo.Name) != "Foo" {
					panic("want Foo")
				}

				implementsBar := doc.Types[person.ImplementsInterfaces.Refs[1]]
				if implementsBar.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(implementsBar.Name) != "Bar" {
					panic("want Bar")
				}

				// field definitions
				nameString := doc.FieldDefinitions[person.FieldsDefinition.Refs[0]]
				name := doc.Input.ByteSliceString(nameString.Name)
				if name != "name" {
					panic("want name")
				}
				nameStringType := doc.Types[nameString.Type]
				if nameStringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				stringName := doc.Input.ByteSliceString(nameStringType.Name)
				if stringName != "String" {
					panic("want String")
				}

				ageField := doc.FieldDefinitions[person.FieldsDefinition.Refs[1]]
				if !ageField.Description.IsDefined {
					panic("want true")
				}
				if ageField.Description.IsBlockString {
					panic("want false	")
				}
				if doc.Input.ByteSliceString(ageField.Description.Content) != "age of the person" {
					panic("want 'age of the person'")
				}
				if doc.Input.ByteSliceString(ageField.Name) != "age" {
					panic("want age")
				}
				intType := doc.Types[ageField.Type]
				if intType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(intType.Name) != "Int" {
					panic("want Int")
				}

				dateOfBirthField := doc.FieldDefinitions[person.FieldsDefinition.Refs[2]]
				if doc.Input.ByteSliceString(dateOfBirthField.Name) != "dateOfBirth" {
					panic("want dateOfBirth")
				}
				if !dateOfBirthField.Description.IsDefined {
					panic("want true")
				}
				if !dateOfBirthField.Description.IsBlockString {
					panic("want true")
				}
				if doc.Input.ByteSliceString(dateOfBirthField.Description.Content) != "date of birth" {
					panic(fmt.Sprintf("want 'date of birth' got: '%s'", doc.Input.ByteSliceString(dateOfBirthField.Description.Content)))
				}
				dateType := doc.Types[dateOfBirthField.Type]
				if doc.Input.ByteSliceString(dateType.Name) != "Date" {
					panic("want Date")
				}
			})
		})
	})
	t.Run("interface type extension", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`extend interface NamedEntity @foo {
 								name: String
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					namedEntity := doc.InterfaceTypeExtensions[0]
					if doc.Input.ByteSliceString(namedEntity.Name) != "NamedEntity" {
						panic("want NamedEntity")
					}

					// fields
					name := doc.FieldDefinitions[namedEntity.FieldsDefinition.Refs[0]]
					if doc.Input.ByteSliceString(name.Name) != "name" {
						panic("want name")
					}

					// directives
					foo := doc.Directives[namedEntity.Directives.Refs[0]]
					if doc.Input.ByteSliceString(foo.Name) != "foo" {
						panic("want foo")
					}
				})
		})
		t.Run("interface implements interface", func(t *testing.T) {
			run(`extend interface NamedEntity implements Foo & Bar {
 								name: String
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					namedEntity := doc.InterfaceTypeExtensions[0]
					if doc.Input.ByteSliceString(namedEntity.Name) != "NamedEntity" {
						panic("want NamedEntity")
					}

					implementsFoo := doc.Types[namedEntity.ImplementsInterfaces.Refs[0]]
					if implementsFoo.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(implementsFoo.Name) != "Foo" {
						panic("want Foo")
					}

					implementsBar := doc.Types[namedEntity.ImplementsInterfaces.Refs[1]]
					if implementsBar.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(implementsBar.Name) != "Bar" {
						panic("want Bar")
					}

					// fields
					name := doc.FieldDefinitions[namedEntity.FieldsDefinition.Refs[0]]
					if doc.Input.ByteSliceString(name.Name) != "name" {
						panic("want name")
					}
				})
		})
	})
	t.Run("scalar type extension", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`extend scalar JSON @foo`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					scalar := doc.ScalarTypeExtensions[0]
					if doc.Input.ByteSliceString(scalar.Name) != "JSON" {
						panic("want JSON")
					}
					foo := doc.Directives[scalar.Directives.Refs[0]]
					if doc.Input.ByteSliceString(foo.Name) != "foo" {
						panic("want foo")
					}
				})
		})
	})
	t.Run("union type extension", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`extend union SearchResult = Photo | Person`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					SearchResult := doc.UnionTypeExtensions[0]

					// union member types

					// Photo
					Photo := doc.Types[SearchResult.UnionMemberTypes.Refs[0]]
					if Photo.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(Photo.Name) != "Photo" {
						panic("want Photo")
					}

					// Person
					Person := doc.Types[SearchResult.UnionMemberTypes.Refs[1]]
					if Person.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(Person.Name) != "Person" {
						panic("want Person")
					}

					// no more types
					if len(SearchResult.UnionMemberTypes.Refs) != 2 {
						t.Fatalf("want 2")
					}
				})
		})
	})
	t.Run("enum type extension", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`extend enum Direction @bar {
							  NORTH
							  EAST
							  SOUTH
							  "describes WEST"
							  WEST @foo
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					direction := doc.EnumTypeExtensions[0]
					if doc.Input.ByteSliceString(direction.Name) != "Direction" {
						panic("want Direction")
					}

					// directives
					bar := doc.Directives[direction.Directives.Refs[0]]
					if doc.Input.ByteSliceString(bar.Name) != "bar" {
						panic("want bar")
					}

					// values

					wantValue := func(index int, name string) {
						enum := doc.EnumValueDefinitions[direction.EnumValuesDefinition.Refs[index]]
						if doc.Input.ByteSliceString(enum.EnumValue) != name {
							panic(fmt.Sprintf("want %s", name))
						}
					}

					wantValue(0, "NORTH")
					wantValue(1, "EAST")
					wantValue(2, "SOUTH")
					wantValue(3, "WEST")

					west := doc.EnumValueDefinitions[direction.EnumValuesDefinition.Refs[3]]
					if !west.Description.IsDefined {
						panic("want true")
					}
					if doc.Input.ByteSliceString(west.Description.Content) != "describes WEST" {
						panic("want describes WEST")
					}

					foo := doc.Directives[west.Directives.Refs[0]]
					if doc.Input.ByteSliceString(foo.Name) != "foo" {
						panic("want foo")
					}

					if len(direction.EnumValuesDefinition.Refs) != 4 {
						panic("want 4")
					}
				})
		})
	})
	t.Run("input object type extension", func(t *testing.T) {
		t.Run("complex", func(t *testing.T) {
			run(`	extend input Person {
									name: String = "Gopher"
								}`, parse,
				false, func(doc *ast.Document, extra interface{}) {
					person := doc.InputObjectTypeExtensions[0]
					if doc.Input.ByteSliceString(person.Name) != "Person" {
						panic("want person")
					}

					name := doc.InputValueDefinitions[person.InputFieldsDefinition.Refs[0]]
					if doc.Input.ByteSliceString(name.Name) != "name" {
						panic("want name")
					}
					if !name.DefaultValue.IsDefined {
						panic("want true")
					}
					if name.DefaultValue.Value.Kind != ast.ValueKindString {
						panic("want ValueKindString")
					}
					if doc.Input.ByteSliceString(doc.StringValues[name.DefaultValue.Value.Ref].Content) != "Gopher" {
						panic("want Gopher")
					}
				})
		})
	})
	t.Run("ignore block string before extend type", func(t *testing.T) {
		run(`"""
		   Schema BlockString to ignore
		   """
		   extend type Schema {
		   }
		   """"
		   Object Type BlockString to ignore
		   """
		   extend type Person {
		   }
		   """"
		   Interface Type BlockString to ignore
		   """
		   extend interface NamedEntity {
		   }
		   """"
		   Scalar Type BlockString to ignore
		   """
		   extend scalar JSON {
		   }
		   """"
		   Union Type BlockString to ignore
		   """
		   extend union SearchResult = Photo | Person {
		   }
		   """"
		   Input Type BlockString to ignore
		   """
		   extend input NamedEntity {
		   }`, parse, false)
	})
	t.Run("scalar type definition", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`scalar JSON`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					scalar := doc.ScalarTypeDefinitions[0]
					if doc.Input.ByteSliceString(scalar.Name) != "JSON" {
						panic("want JSON")
					}
				})
		})
		t.Run("with description", func(t *testing.T) {
			run(`"JSON scalar description" scalar JSON`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					scalar := doc.ScalarTypeDefinitions[0]
					if doc.Input.ByteSliceString(scalar.Name) != "JSON" {
						panic("want JSON")
					}
					if !scalar.Description.IsDefined {
						panic("want true")
					}
					if doc.Input.ByteSliceString(scalar.Description.Content) != "JSON scalar description" {
						panic("want 'JSON scalar description'")
					}
				})
		})
		t.Run("with directive", func(t *testing.T) {
			run(`scalar JSON @foo`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					scalar := doc.ScalarTypeDefinitions[0]
					if doc.Input.ByteSliceString(scalar.Name) != "JSON" {
						panic("want JSON")
					}
					foo := doc.Directives[scalar.Directives.Refs[0]]
					if doc.Input.ByteSliceString(foo.Name) != "foo" {
						panic("want foo")
					}
				})
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
						}`, parse, false, func(doc *ast.Document, extra interface{}) {
				person := doc.ObjectTypeDefinitions[0]
				personName := doc.Input.ByteSliceString(person.Name)
				if personName != "Person" {
					panic("want person")
				}

				// interfaces

				implementsFoo := doc.Types[person.ImplementsInterfaces.Refs[0]]
				if implementsFoo.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(implementsFoo.Name) != "Foo" {
					panic("want Foo")
				}

				implementsBar := doc.Types[person.ImplementsInterfaces.Refs[1]]
				if implementsBar.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(implementsBar.Name) != "Bar" {
					panic("want Bar")
				}

				// field definitions
				nameString := doc.FieldDefinitions[person.FieldsDefinition.Refs[0]]
				name := doc.Input.ByteSliceString(nameString.Name)
				if name != "name" {
					panic("want name")
				}
				nameStringType := doc.Types[nameString.Type]
				if nameStringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				stringName := doc.Input.ByteSliceString(nameStringType.Name)
				if stringName != "String" {
					panic("want String")
				}

				ageField := doc.FieldDefinitions[person.FieldsDefinition.Refs[1]]
				if !ageField.Description.IsDefined {
					panic("want true")
				}
				if ageField.Description.IsBlockString {
					panic("want false	")
				}
				if doc.Input.ByteSliceString(ageField.Description.Content) != "age of the person" {
					panic("want 'age of the person'")
				}
				if doc.Input.ByteSliceString(ageField.Name) != "age" {
					panic("want age")
				}
				intType := doc.Types[ageField.Type]
				if intType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(intType.Name) != "Int" {
					panic("want Int")
				}

				dateOfBirthField := doc.FieldDefinitions[person.FieldsDefinition.Refs[2]]
				if doc.Input.ByteSliceString(dateOfBirthField.Name) != "dateOfBirth" {
					panic("want dateOfBirth")
				}
				if !dateOfBirthField.Description.IsDefined {
					panic("want true")
				}
				if !dateOfBirthField.Description.IsBlockString {
					panic("want true")
				}
				if doc.Input.ByteSliceString(dateOfBirthField.Description.Content) != "date of birth" {
					panic(fmt.Sprintf("want 'date of birth' got: '%s'", doc.Input.ByteSliceString(dateOfBirthField.Description.Content)))
				}
				dateType := doc.Types[dateOfBirthField.Type]
				if doc.Input.ByteSliceString(dateType.Name) != "Date" {
					panic("want Date")
				}
			})
		})
		t.Run("with directives", func(t *testing.T) {
			run(`type Person @foo @bar {}`, parse, false, func(doc *ast.Document, extra interface{}) {
				person := doc.ObjectTypeDefinitions[0]
				personName := doc.Input.ByteSliceString(person.Name)
				if personName != "Person" {
					panic("want person")
				}

				// directives

				foo := doc.Directives[person.Directives.Refs[0]]
				if doc.Input.ByteSliceString(foo.Name) != "foo" {
					panic("want foo")
				}

				bar := doc.Directives[person.Directives.Refs[1]]
				if doc.Input.ByteSliceString(bar.Name) != "bar" {
					panic("want bar")
				}
			})
		})
		t.Run("implements optional variant", func(t *testing.T) {
			run(`type Person implements & Foo & Bar {}`, parse, false, func(doc *ast.Document, extra interface{}) {
				person := doc.ObjectTypeDefinitions[0]
				personName := doc.Input.ByteSliceString(person.Name)
				if personName != "Person" {
					panic("want person")
				}
				// interfaces

				implementsFoo := doc.Types[person.ImplementsInterfaces.Refs[0]]
				if implementsFoo.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(implementsFoo.Name) != "Foo" {
					panic("want Foo")
				}

				implementsBar := doc.Types[person.ImplementsInterfaces.Refs[1]]
				if implementsBar.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(implementsBar.Name) != "Bar" {
					panic("want Bar")
				}
			})
		})
		t.Run("input value definition list", func(t *testing.T) {
			run(`	type Person { 
									"name description"
									name(
										a: String!
										"b description"
										b: Int
										"""
										c description
										"""
										c: Float
									): String
								}`, parse, false, func(doc *ast.Document, extra interface{}) {
				person := doc.ObjectTypeDefinitions[0]
				personName := doc.Input.ByteSliceString(person.Name)
				if personName != "Person" {
					panic("want person")
				}

				name := doc.FieldDefinitions[person.FieldsDefinition.Refs[0]]
				if !name.Description.IsDefined {
					panic("want true")
				}
				if doc.Input.ByteSliceString(name.Description.Content) != "name description" {
					panic("want 'name description'")
				}

				// a

				a := doc.InputValueDefinitions[name.ArgumentsDefinition.Refs[0]]
				if doc.Input.ByteSliceString(a.Name) != "a" {
					panic("want a")
				}
				if doc.Types[a.Type].TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(doc.Types[doc.Types[a.Type].OfType].Name) != "String" {
					panic("want String")
				}

				// b

				b := doc.InputValueDefinitions[name.ArgumentsDefinition.Refs[1]]
				if doc.Input.ByteSliceString(b.Name) != "b" {
					panic("want b")
				}
				if doc.Input.ByteSliceString(b.Description.Content) != "b description" {
					panic("want 'b description'")
				}
				if doc.Types[b.Type].TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(doc.Types[b.Type].Name) != "Int" {
					panic("want Float")
				}

				// c

				c := doc.InputValueDefinitions[name.ArgumentsDefinition.Refs[2]]
				if doc.Input.ByteSliceString(c.Name) != "c" {
					panic("want b")
				}
				if !c.Description.IsDefined {
					panic("want true")
				}
				if !c.Description.IsBlockString {
					panic("want true")
				}
				if doc.Input.ByteSliceString(c.Description.Content) != "c description" {
					panic("want 'c description'")
				}
				if doc.Types[c.Type].TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(doc.Types[c.Type].Name) != "Float" {
					panic("want Float")
				}
			})
		})
		t.Run("object type with commented out field", func(t *testing.T) {
			run(`type Person {
							   name: String
							   # tbd: String
							   age: Int
							}`, parse, false, func(doc *ast.Document, extra interface{}) {
				person := doc.ObjectTypeDefinitions[0]
				personName := doc.Input.ByteSliceString(person.Name)
				if personName != "Person" {
					t.Fatal("want person")
				}

				// field definitions
				if len(person.FieldsDefinition.Refs) != 2 {
					t.Fatal("want 2")
				}
				nameString := doc.FieldDefinitions[person.FieldsDefinition.Refs[0]]
				name := doc.Input.ByteSliceString(nameString.Name)
				if name != "name" {
					t.Fatal("want name")
				}
				nameStringType := doc.Types[nameString.Type]
				if nameStringType.TypeKind != ast.TypeKindNamed {
					t.Fatal("want TypeKindNamed")
				}
				stringName := doc.Input.ByteSliceString(nameStringType.Name)
				if stringName != "String" {
					t.Fatal("want String")
				}
				ageInt := doc.FieldDefinitions[person.FieldsDefinition.Refs[1]]
				age := doc.Input.ByteSliceString(ageInt.Name)
				if age != "age" {
					t.Fatal("want age")
				}
				ageIntType := doc.Types[ageInt.Type]
				if ageIntType.TypeKind != ast.TypeKindNamed {
					t.Fatal("want TypeKindNamed")
				}
				intAge := doc.Input.ByteSliceString(ageIntType.Name)
				if intAge != "Int" {
					t.Fatal("want String")
				}
			})
		})
		t.Run("implements & without next", func(t *testing.T) {
			run(`type Person implements Foo & {}`, parse, true)
		})
	})
	t.Run("input object type definition", func(t *testing.T) {
		t.Run("complex", func(t *testing.T) {
			run(`	input Person {
									name: String = "Gopher"
								}`, parse,
				false, func(doc *ast.Document, extra interface{}) {
					person := doc.InputObjectTypeDefinitions[0]
					if doc.Input.ByteSliceString(person.Name) != "Person" {
						panic("want person")
					}

					name := doc.InputValueDefinitions[person.InputFieldsDefinition.Refs[0]]
					if doc.Input.ByteSliceString(name.Name) != "name" {
						panic("want name")
					}
					if !name.DefaultValue.IsDefined {
						panic("want true")
					}
					if name.DefaultValue.Value.Kind != ast.ValueKindString {
						panic("want ValueKindString")
					}
					if doc.Input.ByteSliceString(doc.StringValues[name.DefaultValue.Value.Ref].Content) != "Gopher" {
						panic("want Gopher")
					}
				})
		})
	})
	t.Run("interface type definition", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`interface NamedEntity @foo {
 								name: String
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					namedEntity := doc.InterfaceTypeDefinitions[0]
					if doc.Input.ByteSliceString(namedEntity.Name) != "NamedEntity" {
						panic("want NamedEntity")
					}

					// fields
					name := doc.FieldDefinitions[namedEntity.FieldsDefinition.Refs[0]]
					if doc.Input.ByteSliceString(name.Name) != "name" {
						panic("want name")
					}

					// directives
					foo := doc.Directives[namedEntity.Directives.Refs[0]]
					if doc.Input.ByteSliceString(foo.Name) != "foo" {
						panic("want foo")
					}
				})
		})
		t.Run("with description", func(t *testing.T) {
			run(`"describes NamedEntity" interface NamedEntity {
 								name: String
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					namedEntity := doc.InterfaceTypeDefinitions[0]
					if doc.Input.ByteSliceString(namedEntity.Name) != "NamedEntity" {
						panic("want NamedEntity")
					}
					if !namedEntity.Description.IsDefined {
						panic("want true")
					}
					if doc.Input.ByteSliceString(namedEntity.Description.Content) != "describes NamedEntity" {
						panic("want 'describes NamedEntity'")
					}
				})
		})
		t.Run("with interface implementation", func(t *testing.T) {
			run(`interface NamedEntity implements Foo & Bar {
 								name: String
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					namedEntity := doc.InterfaceTypeDefinitions[0]
					if doc.Input.ByteSliceString(namedEntity.Name) != "NamedEntity" {
						panic("want NamedEntity")
					}
					implementsFoo := doc.Types[namedEntity.ImplementsInterfaces.Refs[0]]
					if implementsFoo.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(implementsFoo.Name) != "Foo" {
						panic("want Foo")
					}

					implementsBar := doc.Types[namedEntity.ImplementsInterfaces.Refs[1]]
					if implementsBar.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(implementsBar.Name) != "Bar" {
						panic("want Bar")
					}
				})
		})
	})
	t.Run("union type definition", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`union SearchResult = Photo | Person`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					SearchResult := doc.UnionTypeDefinitions[0]

					// union member types

					// Photo
					Photo := doc.Types[SearchResult.UnionMemberTypes.Refs[0]]
					if Photo.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(Photo.Name) != "Photo" {
						panic("want Photo")
					}

					// Person
					Person := doc.Types[SearchResult.UnionMemberTypes.Refs[1]]
					if Person.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(Person.Name) != "Person" {
						panic("want Person")
					}

					// no more types
					if len(SearchResult.UnionMemberTypes.Refs) != 2 {
						panic("want 2")
					}
				})
		})
		t.Run("without members", func(t *testing.T) {
			run(`union SearchResult`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					SearchResult := doc.UnionTypeDefinitions[0]

					// union member types

					// no more types
					if len(SearchResult.UnionMemberTypes.Refs) != 0 {
						panic("want 0")
					}
				})
		})
		t.Run("member missing after pipe", func(t *testing.T) {
			run(`union SearchResult = Photo |`, parse, true)
		})
		t.Run("without members", func(t *testing.T) {
			run(`union SearchResult =`, parse, true)
		})
	})
	t.Run("type", func(t *testing.T) {
		t.Run("named", func(t *testing.T) {
			run("String", parseType, false, func(doc *ast.Document, extra interface{}) {
				stringType := doc.Types[0]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null named", func(t *testing.T) {
			run("String!", parseType, false, func(doc *ast.Document, extra interface{}) {
				nonNull := doc.Types[1]
				if nonNull.TypeKind != ast.TypeKindNonNull {
					panic("want TypeKindNonNull")
				}
				stringType := doc.Types[nonNull.OfType]
				if stringType.TypeKind != ast.TypeKindNamed {
					panic("want TypeKindNamed")
				}
				if doc.Input.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of named", func(t *testing.T) {
			run("[String]!", parseType, false, func(doc *ast.Document, extra interface{}) {
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
				if doc.Input.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of non null named", func(t *testing.T) {
			run("[String!]!", parseType, false, func(doc *ast.Document, extra interface{}) {
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
				if doc.Input.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of non null list of named", func(t *testing.T) {
			run("[[String]!]!", parseType, false, func(doc *ast.Document, extra interface{}) {
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
				if doc.Input.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("non null list of non null list of non null named", func(t *testing.T) {
			run("[[String!]!]!", parseType, false, func(doc *ast.Document, extra interface{}) {
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
				if doc.Input.ByteSliceString(stringType.Name) != "String" {
					panic("want String")
				}
			})
		})
		t.Run("report unexpected bang", func(t *testing.T) {
			run("!", parseType, true)
		})
		t.Run("report empty list", func(t *testing.T) {
			run("[]", parseType, true)
		})
		t.Run("report incomplete list", func(t *testing.T) {
			run("[", parseType, true)
		})
		t.Run("report unclosed list", func(t *testing.T) {
			run("[String", parseType, true)
		})
		t.Run("report unclosed list with bang", func(t *testing.T) {
			run("[String!", parseType, true)
		})
		t.Run("report double bang", func(t *testing.T) {
			run("String!!", parseType, true)
		})
		t.Run("report list close at beginning", func(t *testing.T) {
			run("]String", parseType, true)
		})
	})
	t.Run("enum type definition", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`"enums"
							enum Direction @bar {
							  NORTH
							  EAST
							  SOUTH
							  "describes WEST"
							  WEST @foo
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					direction := doc.EnumTypeDefinitions[0]
					if doc.Input.ByteSliceString(direction.Name) != "Direction" {
						panic("want Direction")
					}

					// directives
					bar := doc.Directives[direction.Directives.Refs[0]]
					if doc.Input.ByteSliceString(bar.Name) != "bar" {
						panic("want bar")
					}

					// values

					wantValue := func(index int, name string) {
						enum := doc.EnumValueDefinitions[direction.EnumValuesDefinition.Refs[index]]
						if doc.Input.ByteSliceString(enum.EnumValue) != name {
							panic(fmt.Sprintf("want %s", name))
						}
					}

					wantValue(0, "NORTH")
					wantValue(1, "EAST")
					wantValue(2, "SOUTH")
					wantValue(3, "WEST")

					west := doc.EnumValueDefinitions[direction.EnumValuesDefinition.Refs[3]]
					if !west.Description.IsDefined {
						panic("want true")
					}
					if doc.Input.ByteSliceString(west.Description.Content) != "describes WEST" {
						panic("want describes WEST")
					}

					foo := doc.Directives[west.Directives.Refs[0]]
					if doc.Input.ByteSliceString(foo.Name) != "foo" {
						panic("want foo")
					}

					if len(direction.EnumValuesDefinition.Refs) != 4 {
						panic("want 4")
					}
				})
		})
	})
	t.Run("directive definition", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`directive @example on FIELD`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					example := doc.DirectiveDefinitions[0]
					if doc.Input.ByteSliceString(example.Name) != "example" {
						panic("want example")
					}
					locations := example.DirectiveLocations.Iterable()
					if !locations.Next() {
						panic("want next")
					}
					if locations.Value() != ast.ExecutableDirectiveLocationField {
						panic("want ExecutableDirectiveLocationField")
					}
					if locations.Next() {
						panic("want false")
					}
				})
		})
		t.Run("repeatable", func(t *testing.T) {
			run(`directive @example repeatable on FIELD`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					example := doc.DirectiveDefinitions[0]
					if doc.Input.ByteSliceString(example.Name) != "example" {
						panic("want example")
					}
					assert.True(t, example.Repeatable.IsRepeatable)
					assert.Equal(t, uint32(20), example.Repeatable.Position.CharStart)
					assert.Equal(t, uint32(30), example.Repeatable.Position.CharEnd)
				})
		})
		t.Run("multiple directive locations", func(t *testing.T) {
			run(`directive @example on FIELD | SCALAR | SCHEMA`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					example := doc.DirectiveDefinitions[0]
					if doc.Input.ByteSliceString(example.Name) != "example" {
						panic("want example")
					}
					locations := example.DirectiveLocations.Iterable()
					if !locations.Next() {
						panic("want next")
					}
					if locations.Value() != ast.ExecutableDirectiveLocationField {
						panic("want ExecutableDirectiveLocationField")
					}
					if !locations.Next() {
						panic("want next")
					}
					if locations.Value() != ast.TypeSystemDirectiveLocationSchema {
						panic("want TypeSystemDirectiveLocationSchema")
					}
					if !locations.Next() {
						panic("want next")
					}
					if locations.Value() != ast.TypeSystemDirectiveLocationScalar {
						panic("want TypeSystemDirectiveLocationScalar")
					}
					if locations.Next() {
						panic("want false")
					}
				})
		})
		t.Run("report pipe at end", func(t *testing.T) {
			run(`directive @example on FIELD | SCALAR | SCHEMA |`, parse, true)
		})
		t.Run("missing location", func(t *testing.T) {
			run(`directive @example on`, parse, true)
		})
		t.Run("invalid location", func(t *testing.T) {
			run(`directive @example on INVALID`, parse, true)
		})
	})
	t.Run("value", func(t *testing.T) {
		t.Run("variable value", func(t *testing.T) {
			t.Run("simple", func(t *testing.T) {
				run(`$foo`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindVariable {
							t.Fatal("want ValueKindVariable")
						}
						foo := doc.VariableValues[value.Ref]
						if doc.Input.ByteSliceString(foo.Name) != "foo" {
							t.Fatal("want foo")
						}
					})
			})
			t.Run("with underscore", func(t *testing.T) {
				run(`$_foo`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindVariable {
							t.Fatal("want ValueKindVariable")
						}
						foo := doc.VariableValues[value.Ref]
						if doc.Input.ByteSliceString(foo.Name) != "_foo" {
							t.Fatal("want foo")
						}
					})
			})
			t.Run("with numbers", func(t *testing.T) {
				run(`$foo123`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindVariable {
							t.Fatal("want ValueKindVariable")
						}
						foo := doc.VariableValues[value.Ref]
						if doc.Input.ByteSliceString(foo.Name) != "foo123" {
							t.Fatal("want foo123")
						}
					})
			})
			t.Run("report space", func(t *testing.T) {
				run(`$ foo`, parseValue, true)
			})
			t.Run("report start with A-Za-z", func(t *testing.T) {
				run(`$123`, parseValue, true)
			})
		})
		t.Run("int value", func(t *testing.T) {
			t.Run("simple", func(t *testing.T) {
				run(`123`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindInteger {
							panic("want ValueKindInteger")
						}
						intValue := doc.IntValues[value.Ref]
						if doc.Input.ByteSliceString(intValue.Raw) != "123" {
							panic("want 123")
						}
						if intValue.Negative {
							panic("want false")
						}
					})
			})
			t.Run("negative", func(t *testing.T) {
				run(`-123`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindInteger {
							panic("want ValueKindInteger")
						}
						intValue := doc.IntValues[value.Ref]
						if doc.Input.ByteSliceString(intValue.Raw) != "123" {
							panic("want 123")
						}
						if !intValue.Negative {
							panic("want false")
						}
					})
			})
			t.Run("report space after negative sign", func(t *testing.T) {
				run(`- 123`, parseValue, true)
			})
		})
		t.Run("float value", func(t *testing.T) {
			t.Run("simple", func(t *testing.T) {
				run(`13.37`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindFloat {
							panic("want ValueKindFloat")
						}
						intValue := doc.FloatValues[value.Ref]
						if doc.Input.ByteSliceString(intValue.Raw) != "13.37" {
							panic("want 13.37")
						}
						if intValue.Negative {
							panic("want false")
						}
					})
			})
			t.Run("negative", func(t *testing.T) {
				run(`-13.37`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindFloat {
							panic("want ValueKindFloat")
						}
						intValue := doc.FloatValues[value.Ref]
						if doc.Input.ByteSliceString(intValue.Raw) != "13.37" {
							panic("want 13.37")
						}
						if !intValue.Negative {
							panic("want false")
						}
					})
			})
			t.Run("report space after negative sign", func(t *testing.T) {
				run(`- 13.37`, parseValue, true)
			})
		})
		t.Run("null value", func(t *testing.T) {
			run(`null`, parseValue, false,
				func(doc *ast.Document, extra interface{}) {
					value := extra.(ast.Value)
					if value.Kind != ast.ValueKindNull {
						panic("want ValueKindNull")
					}
				})
		})
		t.Run("list value", func(t *testing.T) {
			t.Run("complex", func(t *testing.T) {
				run(`[1,2,"3",[4]]`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindList {
							panic("want ValueKindList")
						}

						list := doc.ListValues[value.Ref]

						// 1
						val := doc.Values[list.Refs[0]]
						if val.Kind != ast.ValueKindInteger {
							panic("want ValueKindInteger")
						}
						if doc.Input.ByteSliceString(doc.IntValues[val.Ref].Raw) != "1" {
							panic("want 1")
						}

						// 2
						val = doc.Values[list.Refs[1]]
						if val.Kind != ast.ValueKindInteger {
							panic("want ValueKindInteger")
						}
						if doc.Input.ByteSliceString(doc.IntValues[val.Ref].Raw) != "2" {
							panic("want 1")
						}

						// "3"
						val = doc.Values[list.Refs[2]]
						if val.Kind != ast.ValueKindString {
							panic("want ValueKindString")
						}
						if doc.Input.ByteSliceString(doc.StringValues[val.Ref].Content) != "3" {
							panic("want 3")
						}

						// [4]
						val = doc.Values[list.Refs[3]]
						if val.Kind != ast.ValueKindList {
							panic("want ValueKindString")
						}
						inner := doc.ListValues[val.Ref]

						four := doc.Values[inner.Refs[0]]
						if four.Kind != ast.ValueKindInteger {
							panic("want ValueKindInteger")
						}
						if doc.Input.ByteSliceString(doc.IntValues[four.Ref].Raw) != "4" {
							panic("want 4")
						}
						if len(inner.Refs) != 1 {
							panic("want 1")
						}

						// no more
						if len(list.Refs) != 4 {
							panic("want 4")
						}
					})
			})
		})
		t.Run("object value", func(t *testing.T) {
			t.Run("complex", func(t *testing.T) {
				run(`{lon: 12.43, lat: -53.211, list: [1] }`, parseValue, false,
					func(doc *ast.Document, extra interface{}) {
						value := extra.(ast.Value)
						if value.Kind != ast.ValueKindObject {
							panic("want ValueKindObject")
						}
						object := doc.ObjectValues[value.Ref]

						// lon
						lon := doc.ObjectFields[object.Refs[0]]
						if doc.Input.ByteSliceString(lon.Name) != "lon" {
							panic("want lon")
						}
						if lon.Value.Kind != ast.ValueKindFloat {
							panic("want float")
						}
						if doc.Input.ByteSliceString(doc.FloatValues[lon.Value.Ref].Raw) != "12.43" {
							panic("want 12.43")
						}

						// lat
						lat := doc.ObjectFields[object.Refs[1]]
						if doc.Input.ByteSliceString(lat.Name) != "lat" {
							panic("want lat")
						}
						if lon.Value.Kind != ast.ValueKindFloat {
							panic("want float")
						}
						if !doc.FloatValues[lat.Value.Ref].Negative {
							panic("want negative")
						}
						if doc.Input.ByteSliceString(doc.FloatValues[lat.Value.Ref].Raw) != "53.211" {
							panic("want 53.211")
						}

						// list
						list := doc.ObjectFields[object.Refs[2]]
						if list.Value.Kind != ast.ValueKindList {
							panic("want ValueKindList")
						}
						listValue := doc.ListValues[list.Value.Ref]
						one := doc.Values[listValue.Refs[0]]
						if doc.Input.ByteSliceString(doc.IntValues[one.Ref].Raw) != "1" {
							panic("want 1")
						}
						if len(listValue.Refs) != 1 {
							panic("want 1")
						}

						if len(object.Refs) != 3 {
							panic("want 3")
						}
					})
			})
		})
	})
	t.Run("operation definition", func(t *testing.T) {
		t.Run("unnamed query", func(t *testing.T) {
			run(`query {field}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					query := doc.OperationDefinitions[0]
					if query.OperationType != ast.OperationTypeQuery {
						panic("want OperationTypeQuery")
					}
					if doc.Input.ByteSliceString(query.Name) != "" {
						panic("want empty string")
					}
					fieldSelection := doc.Selections[doc.SelectionSets[query.SelectionSet].SelectionRefs[0]]
					if fieldSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					field := doc.Fields[fieldSelection.Ref]
					if doc.Input.ByteSliceString(field.Name) != "field" {
						panic("want field")
					}
					if field.Position.LineStart != 1 || field.Position.CharStart != 8 {
						panic("want correct position")
					}
				})
		})
		t.Run("shorthand query", func(t *testing.T) {
			run(`{field}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					query := doc.OperationDefinitions[0]
					if query.OperationType != ast.OperationTypeQuery {
						panic("want OperationTypeQuery")
					}
					if doc.Input.ByteSliceString(query.Name) != "" {
						panic("want empty string")
					}
					fieldSelection := doc.Selections[doc.SelectionSets[query.SelectionSet].SelectionRefs[0]]
					if fieldSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					field := doc.Fields[fieldSelection.Ref]
					if doc.Input.ByteSliceString(field.Name) != "field" {
						panic("want field")
					}
				})
		})
		t.Run("named query", func(t *testing.T) {
			run(`query Query1 {field}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					query := doc.OperationDefinitions[0]
					if query.OperationType != ast.OperationTypeQuery {
						panic("want OperationTypeQuery")
					}
					if doc.Input.ByteSliceString(query.Name) != "Query1" {
						panic("want Query1")
					}
					fieldSelection := doc.Selections[doc.SelectionSets[query.SelectionSet].SelectionRefs[0]]
					if fieldSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					field := doc.Fields[fieldSelection.Ref]
					if doc.Input.ByteSliceString(field.Name) != "field" {
						panic("want field")
					}
				})
		})
		t.Run("unnamed mutation", func(t *testing.T) {
			run(`mutation {field}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					mutation := doc.OperationDefinitions[0]
					if mutation.OperationType != ast.OperationTypeMutation {
						panic("want OperationTypeMutation")
					}
					if doc.Input.ByteSliceString(mutation.Name) != "" {
						panic("want empty string")
					}
					fieldSelection := doc.Selections[doc.SelectionSets[mutation.SelectionSet].SelectionRefs[0]]
					if fieldSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					field := doc.Fields[fieldSelection.Ref]
					if doc.Input.ByteSliceString(field.Name) != "field" {
						panic("want field")
					}
				})
		})
		t.Run("named mutation", func(t *testing.T) {
			run(`mutation Mutation1 {field}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					mutation := doc.OperationDefinitions[0]
					if mutation.OperationType != ast.OperationTypeMutation {
						panic("want OperationTypeMutation")
					}
					if doc.Input.ByteSliceString(mutation.Name) != "Mutation1" {
						panic("want Mutation1")
					}
					fieldSelection := doc.Selections[doc.SelectionSets[mutation.SelectionSet].SelectionRefs[0]]
					if fieldSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					field := doc.Fields[fieldSelection.Ref]
					if doc.Input.ByteSliceString(field.Name) != "field" {
						panic("want field")
					}
				})
		})
		t.Run("unnamed subscription", func(t *testing.T) {
			run(`subscription {field}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					mutation := doc.OperationDefinitions[0]
					if mutation.OperationType != ast.OperationTypeSubscription {
						panic("want OperationTypeSubscription")
					}
					if doc.Input.ByteSliceString(mutation.Name) != "" {
						panic("want empty string")
					}
					fieldSelection := doc.Selections[doc.SelectionSets[mutation.SelectionSet].SelectionRefs[0]]
					if fieldSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					field := doc.Fields[fieldSelection.Ref]
					if doc.Input.ByteSliceString(field.Name) != "field" {
						panic("want field")
					}
				})
		})
		t.Run("named subscription", func(t *testing.T) {
			run(`subscription Sub1 {field}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					mutation := doc.OperationDefinitions[0]
					if mutation.OperationType != ast.OperationTypeSubscription {
						panic("want OperationTypeSubscription")
					}
					if doc.Input.ByteSliceString(mutation.Name) != "Sub1" {
						panic("want empty Sub1")
					}
					fieldSelection := doc.Selections[doc.SelectionSets[mutation.SelectionSet].SelectionRefs[0]]
					if fieldSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					field := doc.Fields[fieldSelection.Ref]
					if doc.Input.ByteSliceString(field.Name) != "field" {
						panic("want field")
					}
				})
		})
		t.Run("complex nested subscription", func(t *testing.T) {
			run(`subscription StoryLikeSubscription($input: StoryLikeSubscribeInput) {
  								storyLikeSubscribe(input: $input) {
									story {
									  likers {
										count
									  }
									  likeSentence {
										text
									  }
									}
								  }
							}`,
				parse, false,
				func(doc *ast.Document, extra interface{}) {
					subscription := doc.OperationDefinitions[0]
					if subscription.OperationType != ast.OperationTypeSubscription {
						panic("want OperationTypeSubscription")
					}
				})
		})

		t.Run("operation with comments everywhere", func(t *testing.T) {
			run(`
				query #comment
				findUser#comment
				(#comment
					$userId#comment
				  :#comment
				  ID#comment
				  !#comment
				  #comment
					)#comment
					{#comment
				  user#comment
					  (#comment
					  id#comment
						:#comment
					$userId#comment
					  #comment
				)#comment
					  #comment
				{#comment
					...#comment
				  UserFields#comment
					... #comment
				  on #comment
				  User#comment
				  {#comment
						email#comment
					}#comment
				  }#comment
				}#comment
				
				fragment #comment
				UserFields #comment
				on #comment
				User#comment
				{#comment
				  id#comment
				  #username#comment
					  role#comment
					}#comment`, parse, false)
		})
	})
	t.Run("variable definition", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`($devicePicSize: Int = 1 $var2: String)`, parseVariableDefinitionList, false,
				func(doc *ast.Document, extra interface{}) {
					list := extra.(ast.VariableDefinitionList)

					var1 := doc.VariableDefinitions[list.Refs[0]]
					devicePicSize := doc.VariableValues[var1.VariableValue.Ref]
					if doc.Input.ByteSliceString(devicePicSize.Name) != "devicePicSize" {
						panic("want devicePicSize")
					}
					Int := doc.Types[var1.Type]
					if Int.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(Int.Name) != "Int" {
						panic("want Int")
					}
					if !var1.DefaultValue.IsDefined {
						panic("want true")
					}
					if var1.DefaultValue.Value.Kind != ast.ValueKindInteger {
						panic("want ValueKindInteger")
					}
					one := doc.IntValues[var1.DefaultValue.Value.Ref]
					if doc.Input.ByteSliceString(one.Raw) != "1" {
						panic("want 1")
					}

					var2 := doc.VariableDefinitions[list.Refs[1]]
					var2Variable := doc.VariableValues[var2.VariableValue.Ref]
					if doc.Input.ByteSliceString(var2Variable.Name) != "var2" {
						panic("want var2")
					}
					String := doc.Types[var2.Type]
					if String.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(String.Name) != "String" {
						panic("want String")
					}

					if len(list.Refs) != 2 {
						panic("want 2")
					}
				})
		})
	})
	t.Run("selection set", func(t *testing.T) {
		t.Run("No 8", func(t *testing.T) {
			run(`{
							  me {
								... on Person @foo {
									personID
								}
								...personFragment @bar
								id
								firstName
								lastName
								birthday {
								  month
								  day
								}
								friends {
								  name
								}
							  }
							}`, parseSelectionSet, false,
				func(doc *ast.Document, extra interface{}) {
					set := doc.SelectionSets[extra.(int)]

					// me
					meSelection := doc.Selections[set.SelectionRefs[0]]
					if meSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					me := doc.Fields[meSelection.Ref]
					if doc.Input.ByteSliceString(me.Name) != "me" {
						panic("want me")
					}

					// ... on Person

					onPersonSelection := doc.Selections[doc.SelectionSets[me.SelectionSet].SelectionRefs[0]]
					if onPersonSelection.Kind != ast.SelectionKindInlineFragment {
						panic("want SelectionKindInlineFragment")
					}
					onPersonFragment := doc.InlineFragments[onPersonSelection.Ref]
					if len(onPersonFragment.Directives.Refs) != 1 {
						panic("want 1")
					}
					Person := doc.Types[onPersonFragment.TypeCondition.Type]
					if doc.Input.ByteSliceString(Person.Name) != "Person" {
						panic("want Person")
					}
					personIdSelection := doc.Selections[doc.SelectionSets[onPersonFragment.SelectionSet].SelectionRefs[0]]
					if personIdSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					personId := doc.Fields[personIdSelection.Ref]
					if doc.Input.ByteSliceString(personId.Name) != "personID" {
						panic("want personID")
					}
					if personId.Position.LineStart != 4 || personId.Position.CharStart != 10 {
						panic("want correct position for a person id field")
					}

					// ...personFragment
					personFragmentSelection := doc.Selections[doc.SelectionSets[me.SelectionSet].SelectionRefs[1]]
					if personFragmentSelection.Kind != ast.SelectionKindFragmentSpread {
						panic("want SelectionKindFragmentSpread")
					}
					personFragment := doc.FragmentSpreads[personFragmentSelection.Ref]
					if doc.Input.ByteSliceString(personFragment.FragmentName) != "personFragment" {
						panic("want personFragment")
					}
					if len(personFragment.Directives.Refs) != 1 {
						panic("want 1")
					}

					// id
					idSelection := doc.Selections[doc.SelectionSets[me.SelectionSet].SelectionRefs[2]]
					if idSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					id := doc.Fields[idSelection.Ref]
					if doc.Input.ByteSliceString(id.Name) != "id" {
						panic("want id")
					}

					// birthday
					birthdaySelection := doc.Selections[doc.SelectionSets[me.SelectionSet].SelectionRefs[5]]
					if birthdaySelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					birthday := doc.Fields[birthdaySelection.Ref]
					if doc.Input.ByteSliceString(birthday.Name) != "birthday" {
						panic("want birthday")
					}

					// month
					monthSelection := doc.Selections[doc.SelectionSets[birthday.SelectionSet].SelectionRefs[0]]
					if monthSelection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					month := doc.Fields[monthSelection.Ref]
					if doc.Input.ByteSliceString(month.Name) != "month" {
						panic("want month")
					}
				})
		})
	})
	t.Run("fragment spread", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`friendFields @foo`, parseFragmentSpread, false,
				func(doc *ast.Document, extra interface{}) {
					fragmentSpread := extra.(ast.FragmentSpread)
					if doc.Input.ByteSliceString(fragmentSpread.FragmentName) != "friendFields" {
						panic("want friendFields")
					}
					if len(fragmentSpread.Directives.Refs) != 1 {
						panic("want 1")
					}
				})
		})
		t.Run("report fragment name must not be on", func(t *testing.T) {
			run(`on`, parseFragmentSpread, true)
		})
	})
	t.Run("inline fragment", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`on User {
							  friends {
								count
							  }
							}`, parseInlineFragment, false,
				func(doc *ast.Document, extra interface{}) {
					fragment := extra.(ast.InlineFragment)
					user := doc.Types[fragment.TypeCondition.Type]
					if user.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(user.Name) != "User" {
						panic("want User")
					}

					selection := doc.Selections[doc.SelectionSets[fragment.SelectionSet].SelectionRefs[0]]
					if selection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					friends := doc.Fields[selection.Ref]
					if doc.Input.ByteSliceString(friends.Name) != "friends" {
						panic("want friends")
					}

					selection = doc.Selections[doc.SelectionSets[friends.SelectionSet].SelectionRefs[0]]
					if selection.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					count := doc.Fields[selection.Ref]
					if doc.Input.ByteSliceString(count.Name) != "count" {
						panic("want count")
					}
				})
		})
	})
	t.Run("fragment definition", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			run(`fragment friendFields on User {
							  id
							  name
							  profilePic(size: 50)
							}`, parse, false,
				func(doc *ast.Document, extra interface{}) {
					fragment := doc.FragmentDefinitions[0]
					if doc.Input.ByteSliceString(fragment.Name) != "friendFields" {
						panic("want friendFields")
					}
					onUser := doc.Types[fragment.TypeCondition.Type]
					if onUser.TypeKind != ast.TypeKindNamed {
						panic("want TypeKindNamed")
					}
					if doc.Input.ByteSliceString(onUser.Name) != "User" {
						panic("want User")
					}

					selection1 := doc.Selections[doc.SelectionSets[fragment.SelectionSet].SelectionRefs[0]]
					if selection1.Kind != ast.SelectionKindField {
						panic("want SelectionKindField")
					}
					id := doc.Fields[selection1.Ref]
					if doc.Input.ByteSliceString(id.Name) != "id" {
						panic("want id")
					}
				})
		})
	})
	t.Run("index", func(t *testing.T) {
		run(`
			schema {
				query: Query
				mutation: Mutation
				subscription: Subscription
			}
			scalar Scalar1 {} 
			scalar Scalar2 {} 
			type Type1 {} 
			type Type2 {}
			interface Iface1 {}
			interface Iface2 {}
			input Input1 {}
			input Input2 {}
			union Union1
			union Union2
			enum Enum1 {}
			enum Enum2 {}
			directive @directive1 on FIELD
			directive @directive2 on FIELD
			`, parse, false,
			func(doc *ast.Document, extra interface{}) {
				if string(doc.Index.QueryTypeName) != "Query" {
					panic("want Query")
				}
				if string(doc.Index.MutationTypeName) != "Mutation" {
					panic("want mutation")
				}
				if string(doc.Index.SubscriptionTypeName) != "Subscription" {
					panic("want Subscription")
				}
			},
			wantIndexedNode("Scalar1", ast.NodeKindScalarTypeDefinition),
			wantIndexedNode("Scalar2", ast.NodeKindScalarTypeDefinition),
			wantIndexedNode("Type1", ast.NodeKindObjectTypeDefinition),
			wantIndexedNode("Type2", ast.NodeKindObjectTypeDefinition),
			wantIndexedNode("Iface1", ast.NodeKindInterfaceTypeDefinition),
			wantIndexedNode("Iface2", ast.NodeKindInterfaceTypeDefinition),
			wantIndexedNode("Input1", ast.NodeKindInputObjectTypeDefinition),
			wantIndexedNode("Input2", ast.NodeKindInputObjectTypeDefinition),
			wantIndexedNode("Union1", ast.NodeKindUnionTypeDefinition),
			wantIndexedNode("Union2", ast.NodeKindUnionTypeDefinition),
			wantIndexedNode("Enum1", ast.NodeKindEnumTypeDefinition),
			wantIndexedNode("Enum2", ast.NodeKindEnumTypeDefinition),
			wantIndexedNode("directive1", ast.NodeKindDirectiveDefinition),
			wantIndexedNode("directive2", ast.NodeKindDirectiveDefinition),
		)
	})
}

func TestErrorReport(t *testing.T) {
	t.Run("missing ident", func(t *testing.T) {
		_, report := ParseGraphqlDocumentString(`
			{
		  		me {
					... on Person @foo {
						personID:
					}
				}
			}
		`)

		if !report.HasErrors() {
			t.Fatalf("want err, got nil")
		}

		want := "external: unexpected token - got: RBRACE want one of: [IDENT], locations: [{Line:6 Column:6}], path: []"
		if report.Error() != want {
			t.Fatalf("want:\n%s\ngot:\n%s\n", want, report.Error())
		}
	})
	t.Run("at instead of on", func(t *testing.T) {
		_, report := ParseGraphqlDocumentString(`
			{
		  		me {
					... on @Person @foo {
						personID
					}
				}
			}
		`)

		if !report.HasErrors() {
			t.Fatalf("want err, got nil")
		}

		want := "external: unexpected token - got: AT want one of: [IDENT], locations: [{Line:4 Column:13}], path: []"
		if report.Error() != want {
			t.Fatalf("want:\n%s\ngot:\n%s\n", want, report.Error())
		}
	})
}

func TestParseStarwars(t *testing.T) {

	starWarsSchema, err := os.ReadFile("./testdata/starwars.schema.graphql")
	if err != nil {
		t.Fatal(err)
	}

	_, report := ParseGraphqlDocumentBytes(starWarsSchema)
	if report.HasErrors() {
		t.Fatal(report)
	}
}

func TestParseTodo(t *testing.T) {

	inputFileName := "./testdata/todo.graphql"
	schema, err := os.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	doc, report := ParseGraphqlDocumentBytes(schema)
	if report.HasErrors() {
		t.Fatal(report)
	}

	_ = doc
}

func BenchmarkParseStarwars(b *testing.B) {

	inputFileName := "./testdata/starwars.schema.graphql"
	starwarsSchema, err := os.ReadFile(inputFileName)
	if err != nil {
		b.Fatal(err)
	}

	doc := ast.NewDocument()
	report := operationreport.Report{}
	parser := NewParser()

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(starwarsSchema)))

	for i := 0; i < b.N; i++ {
		doc.Reset()
		doc.Input.ResetInputBytes(starwarsSchema)
		report.Reset()
		parser.Parse(doc, &report)
		if report.HasErrors() {
			b.Fatal(report.Error())
		}
	}
}

func BenchmarkParseGithub(b *testing.B) {

	inputFileName := "./testdata/github.schema.graphql"
	schemaFile, err := os.ReadFile(inputFileName)
	if err != nil {
		b.Fatal(err)
	}

	doc := ast.NewDocument()
	report := operationreport.Report{}
	parser := NewParser()

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(schemaFile)))

	for i := 0; i < b.N; i++ {
		doc.Reset()
		doc.Input.ResetInputBytes(schemaFile)
		parser.Parse(doc, &report)
		if report.HasErrors() {
			b.Fatal(report.Error())
		}
	}
}

func BenchmarkSelectionSet(b *testing.B) {

	doc := ast.NewDocument()
	parser := NewParser()
	report := operationreport.Report{}

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(selectionSet)))

	for i := 0; i < b.N; i++ {
		doc.Reset()
		doc.Input.ResetInputBytes(selectionSet)
		report.Reset()
		parser.Parse(doc, &report)
		if report.HasErrors() {
			b.Fatal(report.Error())
		}
	}
}

func BenchmarkIntrospectionQuery(b *testing.B) {

	doc := ast.NewDocument()
	parser := NewParser()
	report := operationreport.Report{}

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(introspectionQuery)))

	for i := 0; i < b.N; i++ {
		doc.Reset()
		doc.Input.ResetInputBytes(introspectionQuery)
		parser.Parse(doc, &report)
		if report.HasErrors() {
			b.Fatal(report.Error())
		}
	}
}

func BenchmarkKitchenSink(b *testing.B) {

	doc := ast.NewDocument()
	parser := NewParser()
	report := operationreport.Report{}

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(kitchenSinkData)))

	for i := 0; i < b.N; i++ {
		doc.Reset()
		doc.Input.ResetInputBytes(kitchenSinkData)
		report.Reset()
		parser.Parse(doc, &report)
	}
}

func BenchmarkParse(b *testing.B) {

	doc := ast.NewDocument()
	parser := NewParser()
	report := operationreport.Report{}

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(inputBytes)))

	for i := 0; i < b.N; i++ {
		doc.Reset()
		doc.Input.ResetInputBytes(inputBytes)
		parser.Parse(doc, &report)
		if report.HasErrors() {
			b.Fatal(report.Error())
		}
	}
}

var selectionSet = []byte(`{
							  me {
								... on Person @foo {
									personID
								}
								...personFragment @bar
								id
								firstName
								lastName
								birthday {
								  month
								  day
								}
								friends {
								  name
								}
							  }
							}`)

var inputBytes = []byte(`	schema @foo @bar(baz: "bal") {
								query: Query
								mutation: Mutation
								subscription: Subscription 
							}

							"""
							Person type description
							"""
							type Person implements Foo & Bar {
								name: String
								"age of the person"
								age: Int
								"""
								date of birth
								"""
								dateOfBirth: Date
							}

							type PersonWithArgs { 
								"name description"
								name(
									a: String!
									"b description"
									b: Int
									"""
									c description
									"""
									c: Float
								): String
							}

							"scalars"
							scalar JSON

							"inputs"
							input Person {
								name: String = "Gopher"
							}

							"unions"
							union SearchResult = Photo | Person

							"interfaces"
							interface NamedEntity @foo {
 								name: String
							}

							"enums"
							enum Direction {
							  NORTH
							  EAST
							  SOUTH
							  WEST
							}

							"directives"
							directive @example on FIELD | SCALAR | SCHEMA

							query MyQuery {
							  me {
								... on Person @foo {
									personID
								}
								...personFragment @bar
								id
								firstName
								lastName
								birthday {
								  month
								  day
								}
								friends {
								  name
								}
							  }
							}
`)

var kitchenSinkData = []byte(`# Copyright (c) 2015-present, Facebook, Inc.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

query queryName($foo: ComplexType, $site: Site = MOBILE) {
  whoever123is: node(id: [123, 456]) {
    id ,
    ... on User @defer {
      field2 {
        id ,
        alias: field1(first:10, after:$foo,) @include(if: $foo) {
          id,
          ...frag
        }
      }
    }
    ... @skip(unless: $foo) {
      id
    }
    ... {
      id
    }
  }
}

mutation likeStory {
  like(story: 123) @defer {
    story {
      id
    }
  }
}

subscription StoryLikeSubscription($input: StoryLikeSubscribeInput) {
  storyLikeSubscribe(input: $input) {
    story {
      likers {
        count
      }
      likeSentence {
        text
      }
    }
  }
}

fragment frag on Friend {
  foo(size: $size, bar: $b, obj: {key: "value", block: """

      block string uses \"""

  """})
}

{
  unnamed(truthy: true, falsey: false, nullish: null),
  query
}
`)

var introspectionQuery = []byte(`query IntrospectionQuery {
  __schema {
    queryType {
      name
    }
    mutationType {
      name
    }
    subscriptionType {
      name
    }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`)
