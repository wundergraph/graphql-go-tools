package jsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
)

// scalarDefinitions contains the basic scalar types that need to be defined for tests
const scalarDefinitions = `
scalar String
scalar Int
scalar Float
scalar Boolean
scalar ID
`

func TestBuildJsonSchema(t *testing.T) {
	t.Run("simple query with input object", func(t *testing.T) {
		// Define schema
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "criteria": {
      "type": "object",
      "properties": {
        "name": {
          "type": "string"
        },
        "department": {
          "type": "string",
          "nullable": true
        },
        "employmentStatus": {
          "type": "string",
          "enum": [
            "FULL_TIME",
            "PART_TIME",
            "CONTRACTOR",
            "INTERN"
          ],
          "nullable": true
        }
      },
      "required": [
        "name"
      ],
      "additionalProperties": false,
      "description": "Input criteria used to search for employees",
      "nullable": false
    }
  },
  "additionalProperties": false,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("query with nested input objects", func(t *testing.T) {
		// Define schema with nested inputs
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
			type Query {
				findEmployees(criteria: SearchInput): [Employee]
			}
			
			type Employee {
				id: ID!
				name: String
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

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "criteria": {
      "type": "object",
      "properties": {
        "name": {
          "type": "string",
          "nullable": true
        },
        "nested": {
          "type": "object",
          "properties": {
            "hasChildren": {
              "type": "boolean",
              "nullable": true
            },
            "maritalStatus": {
              "type": "string",
              "enum": [
                "MARRIED",
                "ENGAGED"
              ],
              "nullable": true
            },
            "nationality": {
              "type": "string",
              "enum": [
                "AMERICAN",
                "DUTCH",
                "ENGLISH",
                "GERMAN",
                "INDIAN",
                "SPANISH",
                "UKRAINIAN"
              ]
            }
          },
          "required": [
            "nationality"
          ],
          "additionalProperties": false,
          "nullable": false
        }
      },
      "required": [
        "nested"
      ],
      "additionalProperties": false,
      "nullable": false
    }
  },
  "required": [
    "criteria"
  ],
  "additionalProperties": false,
  "nullable": false
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("query with default values", func(t *testing.T) {
		// Define schema with default values
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
			type Query {
				getItems(filter: FilterInput): [Item]
			}
			
			type Item {
				id: ID
				name: String
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
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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
		definitionDoc, report1 := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report1.HasErrors(), "operation parsing failed")

		operationDoc, report2 := astparser.ParseGraphqlDocumentString(operationSDL)
		require.False(t, report2.HasErrors(), "operation parsing failed")

		// Build should return error because type is not defined
		builder := NewVariablesSchemaBuilder(&operationDoc, &definitionDoc)

		// Try to build schema for operation with undefined type
		_, err := builder.Build()
		assert.Error(t, err)
	})

	t.Run("comprehensive test for required arguments", func(t *testing.T) {
		// Define schema with various required and optional fields
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "input": {
      "type": "object",
      "properties": {
        "field1": {
          "type": "string",
          "nullable": true
        },
        "nested": {
          "type": "object",
          "properties": {
            "field2": {
              "type": "boolean",
              "nullable": true
            },
            "deeper": {
              "type": "object",
              "properties": {
                "field3": {
                  "type": "number",
                  "nullable": true
                },
                "enumField": {
                  "type": "string",
                  "enum": [
                    "OPTION_1",
                    "OPTION_2",
                    "OPTION_3"
                  ]
                },
                "arrayOfArrays": {
                  "type": "array",
                  "items": {
                    "type": "array",
                    "items": {
                      "type": "string"
                    }
                  },
                  "nullable": true
                }
              },
              "required": [
                "enumField"
              ],
              "additionalProperties": false,
              "description": "Level 3 input description",
              "nullable": false
            },
            "arrayOfObjects": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "field3": {
                    "type": "number",
                    "nullable": true
                  },
                  "enumField": {
                    "type": "string",
                    "enum": [
                      "OPTION_1",
                      "OPTION_2",
                      "OPTION_3"
                    ]
                  },
                  "arrayOfArrays": {
                    "type": "array",
                    "items": {
                      "type": "array",
                      "items": {
                        "type": "string"
                      }
                    },
                    "nullable": true
                  }
                },
                "required": [
                  "enumField"
                ],
                "additionalProperties": false,
                "description": "Level 3 input description",
                "nullable": true
              },
              "nullable": true
            }
          },
          "required": [
            "deeper"
          ],
          "additionalProperties": false,
          "description": "Level 2 input description",
          "nullable": false
        },
        "optionalArray": {
          "type": "array",
          "items": {
            "type": "string",
            "nullable": true
          },
          "nullable": true
        },
        "requiredArray": {
          "type": "array",
          "items": {
            "type": "integer",
            "nullable": true
          }
        }
      },
      "required": [
        "nested",
        "requiredArray"
      ],
      "additionalProperties": false,
      "description": "Level 1 input description",
      "nullable": false
    }
  },
  "required": [
    "input"
  ],
  "additionalProperties": false,
  "nullable": false
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("recursive types with default recursion depth", func(t *testing.T) {
		// Define schema with recursive input type
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
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

		// Define expected JSON schema - this may vary based on recursion depth setting
		expectedJSON := `{
  "type": "object",
  "properties": {
    "a": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string"
        },
        "name": {
          "type": "string",
          "nullable": true
        },
        "b": {
          "type": "object",
          "properties": {
            "id": {
              "type": "string"
            },
            "description": {
              "type": "string",
              "nullable": true
            }
          },
          "required": [
            "id"
          ],
          "additionalProperties": false,
          "nullable": true
        }
      },
      "required": [
        "id"
      ],
      "additionalProperties": false,
      "nullable": false
    }
  },
  "required": [
    "a"
  ],
  "additionalProperties": false,
  "nullable": false
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("correctly handles nullable and non-nullable fields", func(t *testing.T) {
		// Define schema with a mix of nullable and non-nullable fields
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
			type Query {
				findUser(input: UserInput): User
			}
			
			type User {
				id: ID!
				name: String
			}
			
			input UserInput {
				id: ID
				name: String!
				age: Int
				tags: [String]
				requiredTags: [String]!
				nonNullTags: [String!]
				requiredNonNullTags: [String!]!
				nested: NestedInput
				requiredNested: NestedInput!
			}
			
			input NestedInput {
				field: String
				requiredField: String!
			}
		`

		// Define operation
		operationSDL := `
			query FindUser($input: UserInput) {
				findUser(input: $input) {
					id
					name
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

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "input": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string",
          "nullable": true
        },
        "name": {
          "type": "string"
        },
        "age": {
          "type": "integer",
          "nullable": true
        },
        "tags": {
          "type": "array",
          "items": {
            "type": "string",
            "nullable": true
          },
          "nullable": true
        },
        "requiredTags": {
          "type": "array",
          "items": {
            "type": "string",
            "nullable": true
          }
        },
        "nonNullTags": {
          "type": "array",
          "items": {
            "type": "string"
          },
          "nullable": true
        },
        "requiredNonNullTags": {
          "type": "array",
          "items": {
            "type": "string"
          }
        },
        "nested": {
          "type": "object",
          "properties": {
            "field": {
              "type": "string",
              "nullable": true
            },
            "requiredField": {
              "type": "string"
            }
          },
          "required": [
            "requiredField"
          ],
          "additionalProperties": false,
          "nullable": true
        },
        "requiredNested": {
          "type": "object",
          "properties": {
            "field": {
              "type": "string",
              "nullable": true
            },
            "requiredField": {
              "type": "string"
            }
          },
          "required": [
            "requiredField"
          ],
          "additionalProperties": false,
          "nullable": false
        }
      },
      "required": [
        "name",
        "requiredTags",
        "requiredNonNullTags",
        "requiredNested"
      ],
      "additionalProperties": false,
      "nullable": false
    }
  },
  "additionalProperties": false,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("root schema nullable based on required arguments", func(t *testing.T) {
		// Define schema with required and optional arguments
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
			type Query {
				findUser(id: ID!, name: String): User
			}
			
			type User {
				id: ID!
				name: String
			}
		`

		// Test case 1: Operation with required argument
		operationWithRequired := `
			query GetUser($id: ID!) {
				findUser(id: $id) {
					id
					name
				}
			}
		`

		// Test case 2: Operation with only optional arguments
		operationOptionalOnly := `
			query GetUserByName($name: String) {
				findUser(id: "fixed-id", name: $name) {
					id
					name
				}
			}
		`

		// Parse schema
		definitionDoc, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors(), "schema parsing failed")

		// Parse and test operation with required argument
		operationDoc1, report := astparser.ParseGraphqlDocumentString(operationWithRequired)
		require.False(t, report.HasErrors(), "operation parsing failed")

		schema1, err := BuildJsonSchema(&operationDoc1, &definitionDoc)
		require.NoError(t, err)

		// Convert to JSON to check nullable field
		data1, err := json.MarshalIndent(schema1, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema for required argument case
		expectedJSON1 := `{
  "type": "object",
  "properties": {
    "id": {
      "type": "string"
    }
  },
  "required": [
    "id"
  ],
  "additionalProperties": false,
  "nullable": false
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON1, string(data1), "Required argument schema does not match expected structure")

		// Parse and test operation with only optional arguments
		operationDoc2, report := astparser.ParseGraphqlDocumentString(operationOptionalOnly)
		require.False(t, report.HasErrors(), "operation parsing failed")

		schema2, err := BuildJsonSchema(&operationDoc2, &definitionDoc)
		require.NoError(t, err)

		// Convert to JSON to check nullable field
		data2, err := json.MarshalIndent(schema2, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema for optional argument case
		expectedJSON2 := `{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "nullable": true
    }
  },
  "additionalProperties": false,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON2, string(data2), "Optional argument schema does not match expected structure")
	})

	t.Run("top-level object fields are not nullable", func(t *testing.T) {
		// Define schema
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
			type Query {
				findEmployees(criteria: SearchInput): [Employee]
			}
			
			type Employee {
				id: ID!
				isAvailable: Boolean
				details: EmployeeDetails
			}
			
			type EmployeeDetails {
				forename: String
				nationality: String
			}
			
			input SearchInput {
				name: String
				department: String
			}
		`

		// Define operation
		operationSDL := `
			query MyEmployees($criteria: SearchInput) {
				findEmployees(criteria: $criteria) {
					id
					isAvailable
					details {
						forename
						nationality
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

		// Serialize schema to JSON to check what's exported
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "criteria": {
      "type": "object",
      "properties": {
        "name": {
          "type": "string",
          "nullable": true
        },
        "department": {
          "type": "string",
          "nullable": true
        }
      },
      "additionalProperties": false,
      "nullable": false
    }
  },
  "additionalProperties": false,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("custom scalar types are represented as objects", func(t *testing.T) {
		// Define schema with custom scalar types
		schemaSDL := scalarDefinitions + `
			schema {
				query: Query
			}
			
			"""ISO-8601 date time format"""
			scalar DateTime
			
			"""JSON object represented as string"""
			scalar JSON
			
			type Query {
				searchEvents(from: DateTime, filter: JSON): [Event]
			}
			
			type Event {
				id: ID!
				timestamp: DateTime
				data: JSON
			}
		`

		// Define operation using custom scalar types
		operationSDL := `
			query SearchEvents($from: DateTime, $filter: JSON) {
				searchEvents(from: $from, filter: $filter) {
					id
					timestamp
					data
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

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "from": {
      "nullable": true,
      "description": "ISO-8601 date time format"
    },
    "filter": {
      "nullable": true,
      "description": "JSON object represented as string"
    }
  },
  "additionalProperties": false,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})
}
