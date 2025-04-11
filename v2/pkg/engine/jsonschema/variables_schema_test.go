package jsonschema

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestBuildJsonSchema(t *testing.T) {
	t.Run("simple query with input object", func(t *testing.T) {
		// Define schema
		schemaSDL := `
			type Query {
				findEmployees(criteria: SearchInput): EmployeeResult
			}
			
			type EmployeeResult {
				details: EmployeeDetails
			}
			
			type EmployeeDetails {
				forename: String
			}
			
			"""Input criteria used to search for employees"""
			input SearchInput {
				name: String!
				department: String
				employmentStatus: EmploymentStatus
			}
			
			enum EmploymentStatus {
				FULL_TIME
				PART_TIME
				CONTRACTOR
				INTERN
			}
		`

		// Define operation
		operationSDL := `
			query MyEmployees($criteria: SearchInput) {
				findEmployees(criteria: $criteria) {
					details {
						forename
					}
				}
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Verify schema structure
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify top-level structure
		assert.Equal(t, "object", parsed["type"])
		properties := parsed["properties"].(map[string]interface{})

		// Verify criteria property
		criteria, ok := properties["criteria"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "object", criteria["type"])
		assert.Equal(t, "Input criteria used to search for employees", criteria["description"])

		// Verify criteria properties
		criteriaProps := criteria["properties"].(map[string]interface{})
		assert.Len(t, criteriaProps, 3)

		name := criteriaProps["name"].(map[string]interface{})
		assert.Equal(t, "string", name["type"])

		department := criteriaProps["department"].(map[string]interface{})
		assert.Equal(t, "string", department["type"])

		status := criteriaProps["employmentStatus"].(map[string]interface{})
		assert.Equal(t, "string", status["type"])
		statusEnum := status["enum"].([]interface{})
		assert.ElementsMatch(t, []interface{}{"FULL_TIME", "PART_TIME", "CONTRACTOR", "INTERN"}, statusEnum)

		// Verify required fields
		criteriaRequired := criteria["required"].([]interface{})
		assert.ElementsMatch(t, []interface{}{"name"}, criteriaRequired)

		// Verify additionalProperties is false
		assert.Equal(t, false, criteria["additionalProperties"])
	})

	t.Run("query with nested input objects", func(t *testing.T) {
		// Define schema with nested inputs
		schemaSDL := `
			type Query {
				findEmployees(criteria: SearchInput): [Employee]
			}
			
			input SearchInput {
				name: String
				nested: NestedInput!
			}
			
			input NestedInput {
				hasChildren: Boolean
				maritalStatus: MaritalStatus
				nationality: Nationality!
			}
			
			enum MaritalStatus {
				MARRIED
				ENGAGED
			}
			
			enum Nationality {
				AMERICAN
				DUTCH
				ENGLISH
				GERMAN
				INDIAN
				SPANISH
				UKRAINIAN
			}
		`

		// Define operation
		operationSDL := `
			query MyEmployees($criteria: SearchInput!) {
				findEmployees(criteria: $criteria) {
					id
				}
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Verify schema structure
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify top-level required fields
		assert.Contains(t, parsed["required"], "criteria")

		// Verify criteria structure
		properties := parsed["properties"].(map[string]interface{})
		criteria := properties["criteria"].(map[string]interface{})
		criteriaProps := criteria["properties"].(map[string]interface{})

		// Verify nested structure
		nested := criteriaProps["nested"].(map[string]interface{})
		assert.Equal(t, "object", nested["type"])

		// Verify nested is required
		criteriaRequired := criteria["required"].([]interface{})
		assert.Contains(t, criteriaRequired, "nested")

		// Verify nested properties
		nestedProps := nested["properties"].(map[string]interface{})
		assert.Len(t, nestedProps, 3)

		// Verify nationality is required in nested
		nestedRequired := nested["required"].([]interface{})
		assert.Contains(t, nestedRequired, "nationality")

		// Verify enum in nested
		nationality := nestedProps["nationality"].(map[string]interface{})
		assert.Equal(t, "string", nationality["type"])
		nationalityEnum := nationality["enum"].([]interface{})
		assert.Len(t, nationalityEnum, 7)

		maritalStatus := nestedProps["maritalStatus"].(map[string]interface{})
		assert.Equal(t, "string", maritalStatus["type"])
		maritalEnum := maritalStatus["enum"].([]interface{})
		assert.ElementsMatch(t, []interface{}{"MARRIED", "ENGAGED"}, maritalEnum)
	})

	t.Run("query with default values", func(t *testing.T) {
		// Define schema with default values
		schemaSDL := `
			type Query {
				getItems(filter: FilterInput): [Item]
			}
			
			input FilterInput {
				limit: Int = 10
				includeDeleted: Boolean = false
				status: Status = ACTIVE
			}
			
			enum Status {
				ACTIVE
				PENDING
				DELETED
			}
		`

		// Define operation
		operationSDL := `
			query GetItems($filter: FilterInput = {limit: 5}) {
				getItems(filter: $filter) {
					id
				}
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Verify schema structure
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify filter property with default values
		properties := parsed["properties"].(map[string]interface{})
		filter := properties["filter"].(map[string]interface{})

		// Verify top-level default value
		assert.Equal(t, map[string]interface{}{"limit": float64(5)}, filter["default"])

		// Verify filter properties
		filterProps := filter["properties"].(map[string]interface{})

		// Verify input object default values
		limit := filterProps["limit"].(map[string]interface{})
		assert.Equal(t, float64(10), limit["default"])

		includeDeleted := filterProps["includeDeleted"].(map[string]interface{})
		assert.Equal(t, false, includeDeleted["default"])

		status := filterProps["status"].(map[string]interface{})
		assert.Equal(t, "ACTIVE", status["default"])
	})

	t.Run("query with scalar arguments", func(t *testing.T) {
		// Define schema with scalar arguments
		schemaSDL := `
			type Query {
				getUser(id: ID!, includeProfile: Boolean): User
			}
			
			type User {
				id: ID!
				name: String
				age: Int
				rating: Float
				active: Boolean
			}
		`

		// Define operation
		operationSDL := `
			query GetUser($id: ID!, $includeProfile: Boolean = true, $age: Int, $rating: Float, $name: String) {
				getUser(id: $id, includeProfile: $includeProfile) {
					id
					name
					age
				}
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Verify schema structure
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify top-level structure
		assert.Equal(t, "object", parsed["type"])
		properties := parsed["properties"].(map[string]interface{})

		// Verify required fields
		required := parsed["required"].([]interface{})
		assert.Contains(t, required, "id")

		// Verify ID property
		id, ok := properties["id"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", id["type"])

		// Verify includeProfile property
		includeProfile, ok := properties["includeProfile"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "boolean", includeProfile["type"])
		assert.Equal(t, true, includeProfile["default"])

		// Verify age property
		age, ok := properties["age"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "integer", age["type"])

		// Verify rating property
		rating, ok := properties["rating"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "number", rating["type"])

		// Verify name property
		name, ok := properties["name"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", name["type"])
	})

	t.Run("operation with field descriptions", func(t *testing.T) {
		// Define schema with field descriptions
		schemaSDL := `
			type Query {
				"""Description for getUser field"""
				getUser(id: ID!): User
				
				"""Description for findUsers field"""
				findUsers(filter: UserFilter): [User]
			}
			
			type User {
				id: ID!
				name: String
			}
			
			input UserFilter {
				name: String
				age: Int
			}
		`

		// Define operation
		operationSDL := `
			query GetUserInfo($id: ID!, $filter: UserFilter) {
				getUser(id: $id) {
					id
					name
				}
				findUsers(filter: $filter) {
					id
				}
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Verify schema root description contains field descriptions
		assert.Contains(t, schema.Description, "Description for getUser field")
		assert.Contains(t, schema.Description, "Description for findUsers field")
	})

	t.Run("error handling for undefined types", func(t *testing.T) {
		// Schema missing SearchInput definition
		schemaSDL := `
			type Query {
				search(input: SearchInput): String
			}
		`

		// Operation using SearchInput
		operationSDL := `
			query Search($input: SearchInput) {
				search(input: $input)
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		report = operationreport.Report{} // Reset report

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build should return error because type is not defined
		builder := NewVariablesSchemaBuilder(&operationDoc, &definitionDoc)

		// Try to build schema for operation with undefined type
		_, err := builder.Build()
		assert.Error(t, err)
	})

	t.Run("comprehensive test for required arguments", func(t *testing.T) {
		// Define schema with various required and optional fields
		schemaSDL := `
			type Query {
				search(requiredArg: String!, optionalArg: Int): SearchResult
			}
			
			type SearchResult {
				id: ID!
			}
			
			input RequiredArgsInput {
				requiredField: String!
				optionalField: Float
				requiredNestedInput: RequiredNestedInput!
				optionalNestedInput: OptionalNestedInput
			}
			
			input RequiredNestedInput {
				requiredInnerField: Boolean!
				optionalInnerField: String
			}
			
			input OptionalNestedInput {
				innerField: Int
			}
		`

		// Define operation
		operationSDL := `
			query Search(
				$requiredArg: String!, 
				$optionalArg: Int, 
				$requiredInput: RequiredArgsInput!, 
				$optionalInput: RequiredArgsInput
			) {
				search(requiredArg: $requiredArg, optionalArg: $optionalArg)
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Verify schema structure
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify top-level required fields
		required, ok := parsed["required"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, required, "requiredArg")
		assert.Contains(t, required, "requiredInput")
		assert.NotContains(t, required, "optionalArg")
		assert.NotContains(t, required, "optionalInput")

		// Verify properties
		properties := parsed["properties"].(map[string]interface{})

		// Check required input structure
		requiredInput := properties["requiredInput"].(map[string]interface{})
		assert.Equal(t, "object", requiredInput["type"])

		// Check required fields within input
		inputRequired := requiredInput["required"].([]interface{})
		assert.Contains(t, inputRequired, "requiredField")
		assert.Contains(t, inputRequired, "requiredNestedInput")
		assert.NotContains(t, inputRequired, "optionalField")
		assert.NotContains(t, inputRequired, "optionalNestedInput")

		// Check nested input structure
		inputProperties := requiredInput["properties"].(map[string]interface{})
		requiredNestedInput := inputProperties["requiredNestedInput"].(map[string]interface{})
		assert.Equal(t, "object", requiredNestedInput["type"])

		// Check required fields within nested input
		nestedRequired := requiredNestedInput["required"].([]interface{})
		assert.Contains(t, nestedRequired, "requiredInnerField")
		assert.NotContains(t, nestedRequired, "optionalInnerField")
	})

	t.Run("deeply nested types with mixed requirements", func(t *testing.T) {
		// Define schema with deeply nested types
		schemaSDL := `
			type Query {
				complexSearch(input: Level1Input): SearchResult
			}
			
			type SearchResult {
				id: ID!
			}
			
			"""Level 1 input description"""
			input Level1Input {
				field1: String
				nested: Level2Input!
				optionalArray: [String]
				requiredArray: [Int]!
			}
			
			"""Level 2 input description"""
			input Level2Input {
				field2: Boolean
				deeper: Level3Input!
				arrayOfObjects: [Level3Input]
			}
			
			"""Level 3 input description"""
			input Level3Input {
				field3: Float
				enumField: DeepEnum!
				arrayOfArrays: [[String!]!]
			}
			
			enum DeepEnum {
				OPTION_1
				OPTION_2
				OPTION_3
			}
		`

		// Define operation
		operationSDL := `
			query DeepSearch($input: Level1Input!) {
				complexSearch(input: $input)
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Verify schema structure
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify input is required
		required := parsed["required"].([]interface{})
		assert.Contains(t, required, "input")

		// Get properties
		properties := parsed["properties"].(map[string]interface{})
		input := properties["input"].(map[string]interface{})

		// Verify level 1 description
		assert.Equal(t, "Level 1 input description", input["description"])

		// Verify level 1 required fields
		level1Required := input["required"].([]interface{})
		assert.Contains(t, level1Required, "nested")
		assert.Contains(t, level1Required, "requiredArray")

		// Verify level 1 properties
		level1Properties := input["properties"].(map[string]interface{})

		// Check array types
		requiredArray := level1Properties["requiredArray"].(map[string]interface{})
		assert.Equal(t, "array", requiredArray["type"])
		assert.Equal(t, "integer", requiredArray["items"].(map[string]interface{})["type"])

		// Verify level 2
		nested := level1Properties["nested"].(map[string]interface{})
		assert.Equal(t, "Level 2 input description", nested["description"])

		// Verify level 2 required fields
		level2Required := nested["required"].([]interface{})
		assert.Contains(t, level2Required, "deeper")

		// Verify level 2 properties
		level2Properties := nested["properties"].(map[string]interface{})

		// Verify level 3
		deeper := level2Properties["deeper"].(map[string]interface{})
		assert.Equal(t, "Level 3 input description", deeper["description"])

		// Verify level 3 required fields
		level3Required := deeper["required"].([]interface{})
		assert.Contains(t, level3Required, "enumField")

		// Verify level 3 properties
		level3Properties := deeper["properties"].(map[string]interface{})

		// Verify enum
		enumField := level3Properties["enumField"].(map[string]interface{})
		assert.Equal(t, "string", enumField["type"])

		enumValues := enumField["enum"].([]interface{})
		assert.Contains(t, enumValues, "OPTION_1")
		assert.Contains(t, enumValues, "OPTION_2")
		assert.Contains(t, enumValues, "OPTION_3")

		// Verify array of arrays
		arrayOfArrays := level3Properties["arrayOfArrays"].(map[string]interface{})
		assert.Equal(t, "array", arrayOfArrays["type"])

		innerArray := arrayOfArrays["items"].(map[string]interface{})
		assert.Equal(t, "array", innerArray["type"])
		assert.Equal(t, "string", innerArray["items"].(map[string]interface{})["type"])
	})

	t.Run("recursive types with default recursion depth", func(t *testing.T) {
		// Define schema with recursive input type
		schemaSDL := `
			type Query {
				processNode(node: RecursiveNode): Boolean
			}
			
			"""A node that can contain child nodes of the same type"""
			input RecursiveNode {
				id: ID!
				name: String
				value: Int
				children: [RecursiveNode]
				parent: RecursiveNode
			}
		`

		// Define operation
		operationSDL := `
			query ProcessTree($node: RecursiveNode!) {
				processNode(node: $node)
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema with default recursion depth (1)
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Verify we got a valid schema back
		require.NotNil(t, schema, "Should have a valid schema")

		// Serialize to JSON to check it's valid
		data, err := json.Marshal(schema)
		require.NoError(t, err)
		require.NotEmpty(t, data, "JSON serialization should not be empty")

		// Parse the JSON to verify it's valid
		var result interface{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err, "Schema should be valid JSON")

		// Basic structure checks
		jsonMap, ok := result.(map[string]interface{})
		require.True(t, ok, "Schema should be a JSON object")

		// Check top-level fields
		assert.Equal(t, "object", jsonMap["type"], "Schema should be an object type")
		assert.Contains(t, jsonMap, "properties", "Schema should have properties")

		// Log the schema for debugging
		t.Logf("Default recursion depth schema: %v", string(data))
	})

	t.Run("recursive types with custom recursion depth", func(t *testing.T) {
		// Define schema with recursive input type
		schemaSDL := `
			type Query {
				processNode(node: RecursiveNode): Boolean
			}
			
			"""A node that can contain child nodes of the same type"""
			input RecursiveNode {
				id: ID!
				name: String
				value: Int
				children: [RecursiveNode]
			}
		`

		// Define operation
		operationSDL := `
			query ProcessTree($node: RecursiveNode!) {
				processNode(node: $node)
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema with custom recursion depth (3)
		customDepth := 3
		customSchema, err := BuildJsonSchemaWithOptions(&operationDoc, &definitionDoc, customDepth)
		require.NoError(t, err)

		// Build JSON schema with default recursion depth (1) for comparison
		defaultSchema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Convert both to JSON for analysis
		customData, err := json.Marshal(customSchema)
		require.NoError(t, err)
		defaultData, err := json.Marshal(defaultSchema)
		require.NoError(t, err)

		// Verify both are valid schemas
		require.NotEmpty(t, customData, "Custom schema JSON should not be empty")
		require.NotEmpty(t, defaultData, "Default schema JSON should not be empty")

		// Verify the custom schema is different (likely larger) than the default
		assert.NotEqual(t, string(customData), string(defaultData),
			"Custom recursion depth schema should differ from default schema")

		// Simple size check - custom schema should be larger due to more recursion
		assert.Greater(t, len(customData), len(defaultData),
			"Custom schema should be larger than default due to deeper recursion")

		// Log the schemas for debugging
		t.Logf("Custom recursion depth schema size: %d bytes", len(customData))
		t.Logf("Default recursion depth schema size: %d bytes", len(defaultData))
	})

	t.Run("query with two nested arguments", func(t *testing.T) {
		// Define schema with two complex input types
		schemaSDL := `
			type Query {
				searchUsers(userFilter: UserFilter, orderBy: OrderByInput): [User]
			}
			
			type User {
				id: ID!
				name: String
				email: String
			}
			
			"""Input for filtering users"""
			input UserFilter {
				nameContains: String
				emailDomain: String
				status: UserStatus
				metadata: MetadataInput
			}
			
			"""Input for ordering results"""
			input OrderByInput {
				field: OrderableField!
				direction: SortDirection!
				nullsPosition: NullsPosition
			}
			
			input MetadataInput {
				tags: [String!]
				createdAfter: String
				createdBefore: String
			}
			
			enum UserStatus {
				ACTIVE
				INACTIVE
				PENDING
			}
			
			enum OrderableField {
				NAME
				EMAIL
				CREATED_AT
				UPDATED_AT
			}
			
			enum SortDirection {
				ASC
				DESC
			}
			
			enum NullsPosition {
				FIRST
				LAST
			}
		`

		// Define operation using both input types
		operationSDL := `
			query FindUsers($filter: UserFilter!, $order: OrderByInput!) {
				searchUsers(userFilter: $filter, orderBy: $order) {
					id
					name
					email
				}
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Verify schema structure
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify top-level structure
		assert.Equal(t, "object", parsed["type"])

		// Verify both inputs are required
		required, ok := parsed["required"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, required, "filter")
		assert.Contains(t, required, "order")

		// Verify properties exist
		properties := parsed["properties"].(map[string]interface{})
		assert.Contains(t, properties, "filter")
		assert.Contains(t, properties, "order")

		// Verify filter structure
		filter := properties["filter"].(map[string]interface{})
		assert.Equal(t, "object", filter["type"])
		assert.Equal(t, "Input for filtering users", filter["description"])
		assert.Contains(t, filter["properties"], "metadata")

		// Verify order structure
		order := properties["order"].(map[string]interface{})
		assert.Equal(t, "object", order["type"])
		assert.Equal(t, "Input for ordering results", order["description"])

		// Verify order required fields
		orderRequired := order["required"].([]interface{})
		assert.Contains(t, orderRequired, "field")
		assert.Contains(t, orderRequired, "direction")

		// Verify enum values
		orderProps := order["properties"].(map[string]interface{})
		direction := orderProps["direction"].(map[string]interface{})
		directionEnum := direction["enum"].([]interface{})
		assert.ElementsMatch(t, []interface{}{"ASC", "DESC"}, directionEnum)
	})

	t.Run("mutually recursive types", func(t *testing.T) {
		// Define schema with mutually recursive input types
		schemaSDL := `
			type Query {
				processA(a: TypeA): Boolean
			}
			
			input TypeA {
				id: ID!
				name: String
				b: TypeB
			}
			
			input TypeB {
				id: ID!
				description: String
				a: TypeA
			}
		`

		// Define operation
		operationSDL := `
			query ProcessA($a: TypeA!) {
				processA(a: $a)
			}
		`

		// Parse schema and operation
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		operationDoc, report := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report.HasErrors(), "operation parsing failed")

		// Build JSON schema
		schema, err := BuildJsonSchema(&operationDoc, &definitionDoc)
		require.NoError(t, err)

		// Serialize schema to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)
		jsonStr := string(data)

		// Check for base structure and non-recursive fields
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify required attribute a exists at the top level
		properties, ok := parsed["properties"].(map[string]interface{})
		require.True(t, ok)
		_, ok = properties["a"].(map[string]interface{})
		require.True(t, ok)

		// Check non-recursive fields in both types are present
		assert.Contains(t, jsonStr, `"id":`)
		assert.Contains(t, jsonStr, `"name":`)
		assert.Contains(t, jsonStr, `"description":`)

		// Verify at least one a or b reference exists (showing some level of recursion was processed)
		assert.True(t, strings.Contains(jsonStr, `"a":`) || strings.Contains(jsonStr, `"b":`),
			"Should have at least one reference to a recursive field")
	})
}
