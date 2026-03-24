package jsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonSchema_MarshalJSON(t *testing.T) {
	t.Run("object schema", func(t *testing.T) {
		// Create a complex nested schema
		schema := NewObjectSchema()
		schema.Description = "Test object schema"

		// Add string property with description and default
		stringProp := NewStringSchema()
		stringProp.Description = "A string property"
		stringProp.Default = "default value"
		schema.Properties["name"] = stringProp
		schema.Required = append(schema.Required, "name")

		// Add integer property with minimum
		intProp := NewIntegerSchema()
		min := float64(0)
		intProp.Minimum = &min
		schema.Properties["age"] = intProp
		schema.Required = append(schema.Required, "age")

		// Add enum property
		enumValues := []string{"ONE", "TWO", "THREE"}
		enumProp := NewEnumSchema(enumValues)
		schema.Properties["category"] = enumProp

		// Add nested object property
		nestedObj := NewObjectSchema()
		nestedObj.Properties["street"] = NewStringSchema()
		nestedObj.Properties["city"] = NewStringSchema()
		nestedObj.Required = append(nestedObj.Required, "street")
		schema.Properties["address"] = nestedObj

		// Add array property
		arrayProp := NewArraySchema(NewStringSchema())
		schema.Properties["tags"] = arrayProp

		// Serialize to JSON
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "A string property",
      "default": "default value",
      "nullable": true
    },
    "age": {
      "type": "integer",
      "minimum": 0,
      "nullable": true
    },
    "category": {
      "type": "string",
      "enum": [
        "ONE",
        "TWO",
        "THREE"
      ],
      "nullable": true
    },
    "address": {
      "type": "object",
      "properties": {
        "street": {
          "type": "string",
          "nullable": true
        },
        "city": {
          "type": "string",
          "nullable": true
        }
      },
      "required": [
        "street"
      ],
      "additionalProperties": false,
      "nullable": true
    },
    "tags": {
      "type": "array",
      "items": {
        "type": "string",
        "nullable": true
      },
      "nullable": true
    }
  },
  "required": [
    "name",
    "age"
  ],
  "additionalProperties": false,
  "description": "Test object schema",
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("nested schema", func(t *testing.T) {
		// Create a schema with nested objects (previously would have used references)
		rootSchema := NewObjectSchema()
		rootSchema.Description = "Root schema"

		// Create a nested schema
		nestedSchema := NewObjectSchema()
		nestedSchema.Description = "Nested schema"
		nestedSchema.Properties["value"] = NewStringSchema()

		// Add the nested schema as a property
		rootSchema.Properties["nested"] = nestedSchema

		// Create an array of the nested schema
		arraySchema := NewArraySchema(nestedSchema)
		rootSchema.Properties["items"] = arraySchema

		// Serialize to JSON
		data, err := json.Marshal(rootSchema)
		require.NoError(t, err)

		// Parse it back to verify
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify structure - there should be no $ref
		properties := parsed["properties"].(map[string]interface{})
		nestedProp := properties["nested"].(map[string]interface{})

		// Check that it's properly inlined
		assert.Equal(t, "object", nestedProp["type"])
		assert.Equal(t, "Nested schema", nestedProp["description"])
		assert.Contains(t, nestedProp, "properties")

		// Check the array contains the same schema inline
		itemsProp := properties["items"].(map[string]interface{})
		assert.Equal(t, "array", itemsProp["type"])
		assert.Contains(t, itemsProp, "items")

		itemsSchema := itemsProp["items"].(map[string]interface{})
		assert.Equal(t, "object", itemsSchema["type"])
		assert.Equal(t, "Nested schema", itemsSchema["description"])
	})
}

func TestSchemaFeatures(t *testing.T) {
	t.Run("enum schema", func(t *testing.T) {
		// Test creating and validating enum schema
		values := []string{"RED", "GREEN", "BLUE"}
		schema := NewEnumSchema(values)

		// Test serialization
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema
		expectedJSON := `{
  "type": "string",
  "enum": [
    "RED",
    "GREEN",
    "BLUE"
  ],
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("required fields", func(t *testing.T) {
		// Create schema with required fields
		schema := NewObjectSchema()
		schema.Properties["id"] = NewStringSchema()
		schema.Properties["name"] = NewStringSchema()
		schema.Properties["age"] = NewIntegerSchema()

		// Mark id and age as required
		schema.Required = []string{"id", "age"}

		// Serialize and check
		data, err := json.MarshalIndent(schema, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "nullable": true
    },
    "name": {
      "type": "string",
      "nullable": true
    },
    "age": {
      "type": "integer",
      "nullable": true
    }
  },
  "required": [
    "id",
    "age"
  ],
  "additionalProperties": false,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("numeric constraints", func(t *testing.T) {
		// Test numeric constraints (min/max)
		min := float64(0)
		max := float64(100)

		// Integer schema
		intSchema := NewIntegerSchema()
		intSchema.Minimum = &min
		intSchema.Maximum = &max

		data, err := json.MarshalIndent(intSchema, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema for integer
		expectedIntJSON := `{
  "type": "integer",
  "minimum": 0,
  "maximum": 100,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedIntJSON, string(data), "Integer schema does not match expected structure")

		// Number schema
		numSchema := NewNumberSchema()
		numSchema.Minimum = &min
		numSchema.Maximum = &max

		data, err = json.MarshalIndent(numSchema, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema for number
		expectedNumJSON := `{
  "type": "number",
  "minimum": 0,
  "maximum": 100,
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedNumJSON, string(data), "Number schema does not match expected structure")
	})

	t.Run("string format", func(t *testing.T) {
		// Test string format
		schema := NewStringSchema()
		schema.Format = "email"

		data, err := json.Marshal(schema)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "email", parsed["format"])
	})

	t.Run("default values", func(t *testing.T) {
		// Test default values for different types
		stringSchema := NewStringSchema()
		stringSchema.Default = "default string"

		intSchema := NewIntegerSchema()
		intSchema.Default = 42

		boolSchema := NewBooleanSchema()
		boolSchema.Default = true

		// Test object with default values
		objSchema := NewObjectSchema()
		objSchema.Properties["str"] = stringSchema
		objSchema.Properties["num"] = intSchema
		objSchema.Properties["bool"] = boolSchema

		data, err := json.Marshal(objSchema)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		properties := parsed["properties"].(map[string]interface{})

		strProp := properties["str"].(map[string]interface{})
		assert.Equal(t, "default string", strProp["default"])

		numProp := properties["num"].(map[string]interface{})
		assert.Equal(t, float64(42), numProp["default"])

		boolProp := properties["bool"].(map[string]interface{})
		assert.Equal(t, true, boolProp["default"])
	})

	t.Run("pattern validation", func(t *testing.T) {
		// Test pattern validation for strings
		schema := NewStringSchema()
		schema.Pattern = "^[a-zA-Z0-9]+$"

		data, err := json.Marshal(schema)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "^[a-zA-Z0-9]+$", parsed["pattern"])
	})

	t.Run("nullable types", func(t *testing.T) {
		// Test all nullable types
		schemas := []*JsonSchema{
			NewObjectSchema(),
			NewArraySchema(NewStringSchema()),
			NewStringSchema(),
			NewIntegerSchema(),
			NewNumberSchema(),
			NewBooleanSchema(),
			NewEnumSchema([]string{"A", "B"}),
		}

		for _, schema := range schemas {
			data, err := json.Marshal(schema)
			require.NoError(t, err)

			var parsed map[string]interface{}
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)

			// For string type assertions, we expect the primary type (without null)
			typeVal := parsed["type"].(string)
			assert.NotEqual(t, "null", typeVal)
		}
	})

	t.Run("fluent interface", func(t *testing.T) {
		// Test fluent interface for building schemas
		schema := NewStringSchema().
			WithDescription("A string with format and default").
			WithFormat("email").
			WithDefault("user@example.com")

		data, err := json.Marshal(schema)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "A string with format and default", parsed["description"])
		assert.Equal(t, "email", parsed["format"])
		assert.Equal(t, "user@example.com", parsed["default"])
	})

	t.Run("complex nested schema", func(t *testing.T) {
		// Test a complex schema with all features
		userSchema := NewObjectSchema()
		userSchema.Description = "User schema with all features"

		// Required string with pattern
		idSchema := NewStringSchema()
		idSchema.Pattern = "^[a-zA-Z0-9]{8,}$"
		userSchema.Properties["id"] = idSchema
		userSchema.Required = append(userSchema.Required, "id")

		// String with format and default
		emailSchema := NewStringSchema()
		emailSchema.Format = "email"
		emailSchema.Default = "user@example.com"
		userSchema.Properties["email"] = emailSchema

		// Integer with constraints
		min := float64(13)
		ageSchema := NewIntegerSchema()
		ageSchema.Minimum = &min
		userSchema.Properties["age"] = ageSchema

		// Enum property
		roleSchema := NewEnumSchema([]string{"ADMIN", "USER", "GUEST"})
		roleSchema.Default = "USER"
		userSchema.Properties["role"] = roleSchema

		// Array of strings
		tagsSchema := NewArraySchema(NewStringSchema())
		userSchema.Properties["tags"] = tagsSchema

		// Nested object
		addressSchema := NewObjectSchema()
		addressSchema.Properties["street"] = NewStringSchema()
		addressSchema.Properties["city"] = NewStringSchema()
		addressSchema.Required = append(addressSchema.Required, "street")
		userSchema.Properties["address"] = addressSchema

		// Serialize the whole thing
		data, err := json.MarshalIndent(userSchema, "", "  ")
		require.NoError(t, err)

		// Define expected JSON schema
		expectedJSON := `{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "pattern": "^[a-zA-Z0-9]{8,}$",
      "nullable": true
    },
    "email": {
      "type": "string",
      "format": "email",
      "default": "user@example.com",
      "nullable": true
    },
    "age": {
      "type": "integer",
      "minimum": 13,
      "nullable": true
    },
    "role": {
      "type": "string",
      "enum": [
        "ADMIN",
        "USER",
        "GUEST"
      ],
      "default": "USER",
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
    "address": {
      "type": "object",
      "properties": {
        "street": {
          "type": "string",
          "nullable": true
        },
        "city": {
          "type": "string",
          "nullable": true
        }
      },
      "required": [
        "street"
      ],
      "additionalProperties": false,
      "nullable": true
    }
  },
  "required": [
    "id"
  ],
  "additionalProperties": false,
  "description": "User schema with all features",
  "nullable": true
}`

		// Compare actual JSON with expected JSON
		assert.JSONEq(t, expectedJSON, string(data), "JSON schema does not match expected structure")
	})

	t.Run("nullable schema property", func(t *testing.T) {
		// Test creating schemas with different nullable settings

		// Create a schema with nullable field
		schema := NewObjectSchema()
		schema.Properties["nullableString"] = NewStringSchema().WithNullable(true)
		schema.Properties["nonNullableString"] = NewStringSchema().WithNullable(false)

		// By default, all types should be nullable
		schema.Properties["defaultString"] = NewStringSchema()

		// Check that factory methods set nullable to true by default
		intSchema := NewIntegerSchema()
		assert.True(t, intSchema.Nullable)

		numSchema := NewNumberSchema()
		assert.True(t, numSchema.Nullable)

		boolSchema := NewBooleanSchema()
		assert.True(t, boolSchema.Nullable)

		enumSchema := NewEnumSchema([]string{"A", "B"})
		assert.True(t, enumSchema.Nullable)

		arraySchema := NewArraySchema(NewStringSchema())
		assert.True(t, arraySchema.Nullable)

		objSchema := NewObjectSchema()
		assert.True(t, objSchema.Nullable)

		// Test serialization
		data, err := json.Marshal(schema)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		properties := parsed["properties"].(map[string]interface{})

		// Explicitly nullable property should have nullable=true
		nullableProp := properties["nullableString"].(map[string]interface{})
		assert.Equal(t, true, nullableProp["nullable"])

		// Non-nullable property should not have nullable field (omitempty)
		nonNullableProp := properties["nonNullableString"].(map[string]interface{})
		_, hasNullable := nonNullableProp["nullable"]
		assert.False(t, hasNullable)

		// Default property should have nullable=true
		defaultProp := properties["defaultString"].(map[string]interface{})
		assert.Equal(t, true, defaultProp["nullable"])

		// Test WithNullable method
		schema = NewStringSchema()
		schema.WithNullable(true)
		assert.True(t, schema.Nullable)

		schema.WithNullable(false)
		assert.False(t, schema.Nullable)
	})
}
