package ast

// Create a new document with initialized slices.
// In case you're on a hot path you always want to use a pre-initialized Document.
func ExampleNewDocument() {

	schema := []byte(`
		schema {
			query: Query
		}
		
		type Query {
			hello: String!
		}
	`)

	doc := NewDocument()
	doc.Input.ResetInputBytes(schema)

	// ...then parse the Input
}

// Create a new Document without pre-initializing slices.
// Use this if you want to manually create a new Document
func ExampleDocument() {

	// create the same doc as in NewDocument() example but manually.

	doc := &Document{}

	// add Query to the raw input
	queryTypeName := doc.Input.AppendInputString("Query")

	// create a RootOperationTypeDefinition
	rootOperationTypeDefinition := RootOperationTypeDefinition{
		OperationType: OperationTypeQuery,
		NamedType: Type{
			Name: queryTypeName,
		},
	}

	// add the RootOperationTypeDefinition to the ast
	doc.RootOperationTypeDefinitions = append(doc.RootOperationTypeDefinitions, rootOperationTypeDefinition)
	// get a reference to the RootOperationTypeDefinition
	queryRootOperationTypeRef := len(doc.RootOperationTypeDefinitions) - 1

	// create a SchemaDefinition
	schemaDefinition := SchemaDefinition{
		RootOperationTypeDefinitions: RootOperationTypeDefinitionList{
			// add the RootOperationTypeDefinition reference
			Refs: []int{queryRootOperationTypeRef},
		},
	}

	// add the SchemaDefinition to the ast
	doc.SchemaDefinitions = append(doc.SchemaDefinitions, schemaDefinition)
	// get a reference to the SchemaDefinition
	schemaDefinitionRef := len(doc.SchemaDefinitions) - 1

	// add the SchemaDefinition to the RootNodes
	// all root level nodes have to be added to the RootNodes slice in order to make them available to the Walker for traversal
	doc.RootNodes = append(doc.RootNodes, Node{Kind: NodeKindSchemaDefinition, Ref: schemaDefinitionRef})

	// add another string to the raw input
	stringName := doc.Input.AppendInputString("String")

	// create a named Type
	stringType := Type{
		TypeKind:TypeKindNamed,
		Name: stringName,
	}

	// add the Type to the ast
	doc.Types = append(doc.Types,stringType)
	// get a reference to the Type
	stringTypeRef := len(doc.Types) -1

	// create another Type
	nonNullStringType := Type{
		TypeKind:TypeKindNonNull,
		// add a reference to the named type
		OfType:stringTypeRef,
	}
	// Result: NonNull String / String!

	// add the Type to the ast
	doc.Types = append(doc.Types,nonNullStringType)
	// get a reference to the Type
	nonNullStringTypeRef := len(doc.Types) -1

	// add another string to the raw input
	helloName := doc.Input.AppendInputString("hello")

	// create a FieldDefinition
	helloFieldDefinition := FieldDefinition{
		Name: helloName,
		// add the Type reference
		Type: nonNullStringTypeRef,
	}

	// add the FieldDefinition to the ast
	doc.FieldDefinitions = append(doc.FieldDefinitions,helloFieldDefinition)
	// get a reference to the FieldDefinition
	helloFieldDefinitionRef := len(doc.FieldDefinitions)-1

	// create an ObjectTypeDefinition
	queryTypeDefinition := ObjectTypeDefinition{
		Name:                queryTypeName,
		// declare that this ObjectTypeDefinition has fields
		// this is necessary for the Walker to understand it must walk FieldDefinitions
		HasFieldDefinitions: true,
		FieldsDefinition: FieldDefinitionList{
			// add the FieldDefinition reference
			Refs: []int{helloFieldDefinitionRef},
		},
	}

	// add ObjectTypeDefinition to the ast
	doc.ObjectTypeDefinitions = append(doc.ObjectTypeDefinitions, queryTypeDefinition)
	// get reference to ObjectTypeDefinition
	queryTypeRef := len(doc.ObjectTypeDefinitions) - 1

	// add ObjectTypeDefinition to the RootNodes
	doc.RootNodes = append(doc.RootNodes, Node{Kind: NodeKindObjectTypeDefinition, Ref: queryTypeRef})
}
