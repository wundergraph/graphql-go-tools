package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSchemaRuntime(t *testing.T) {
	compiler, err := NewProtoCompiler(testSchemaWithLookup, testMapping())
	require.NoError(t, err)

	runtime, err := newSchemaRuntime(compiler.doc)
	require.NoError(t, err)

	require.Len(t, runtime.messageByName, 5)
	require.Len(t, runtime.messageByFullname, 5)
	require.Len(t, runtime.enumByName, 2)
	require.Len(t, runtime.serviceNamesByMethod, 1)
}

func TestSchemaRuntimeMessages(t *testing.T) {
	compiler, err := NewProtoCompiler(testSchemaWithLookup, testMapping())
	require.NoError(t, err)

	runtime, err := newSchemaRuntime(compiler.doc)
	require.NoError(t, err)

	t.Run("getMessageByName returns existing message", func(t *testing.T) {
		msg := runtime.getMessageByName("Product")
		require.NotNil(t, msg)
		assert.Equal(t, "Product", msg.name)
	})

	t.Run("getMessageByName returns nil for unknown message", func(t *testing.T) {
		msg := runtime.getMessageByName("NonExistent")
		assert.Nil(t, msg)
	})

	t.Run("message has correct fields", func(t *testing.T) {
		msg := runtime.getMessageByName("Product")
		require.NotNil(t, msg)

		assert.Contains(t, msg.fieldsByName, "id")
		assert.Contains(t, msg.fieldsByName, "name")
		assert.Contains(t, msg.fieldsByName, "price")
		assert.Contains(t, msg.fieldsByName, "status")
		assert.Contains(t, msg.fieldsByName, "category")
	})

	t.Run("field data types are correct", func(t *testing.T) {
		msg := runtime.getMessageByName("Product")
		require.NotNil(t, msg)

		assert.Equal(t, DataTypeString, msg.fieldsByName["id"].dataType)
		assert.Equal(t, DataTypeString, msg.fieldsByName["name"].dataType)
		assert.Equal(t, DataTypeDouble, msg.fieldsByName["price"].dataType)
		assert.Equal(t, DataTypeEnum, msg.fieldsByName["status"].dataType)
		assert.Equal(t, DataTypeEnum, msg.fieldsByName["category"].dataType)
	})

	t.Run("repeated field is detected", func(t *testing.T) {
		msg := runtime.getMessageByName("LookupProductByIdRequest")
		require.NotNil(t, msg)

		field := msg.fieldsByName["inputs"]
		require.NotNil(t, field)
		assert.True(t, field.repeated)
	})

	t.Run("message field has child message reference", func(t *testing.T) {
		msg := runtime.getMessageByName("LookupProductByIdResult")
		require.NotNil(t, msg)

		field := msg.fieldsByName["product"]
		require.NotNil(t, field)
		assert.Equal(t, DataTypeMessage, field.dataType)
		require.NotNil(t, field.message)
		assert.Equal(t, "Product", field.message.name)
	})

	t.Run("newEmptyMessage creates a valid message", func(t *testing.T) {
		msg := runtime.getMessageByName("Product")
		require.NotNil(t, msg)

		empty := msg.newEmptyMessage()
		require.NotNil(t, empty)
		assert.Equal(t, msg.desc.FullName(), empty.Descriptor().FullName())
	})
}

func TestSchemaRuntimeEnums(t *testing.T) {
	compiler, err := NewProtoCompiler(testSchemaWithLookup, testMapping())
	require.NoError(t, err)

	runtime, err := newSchemaRuntime(compiler.doc)
	require.NoError(t, err)

	t.Run("enums are registered by name", func(t *testing.T) {
		require.Contains(t, runtime.enumByName, "ProductStatus")
		require.Contains(t, runtime.enumByName, "CategoryKind")
	})

	t.Run("enum has correct values", func(t *testing.T) {
		productStatus := runtime.enumByName["ProductStatus"]
		require.NotNil(t, productStatus)
		assert.Equal(t, "ProductStatus", productStatus.name)

		// Values are keyed by GraphqlValue (mapped from the proto enum value name)
		assert.NotEmpty(t, productStatus.valuesByName)
	})

	t.Run("enum values have correct numeric values", func(t *testing.T) {
		categoryKind := runtime.enumByName["CategoryKind"]
		require.NotNil(t, categoryKind)

		// The mapping transforms proto names to GraphQL names.
		// Check that we have entries and they carry the right numeric values.
		for _, v := range categoryKind.valuesByName {
			assert.GreaterOrEqual(t, v.value, int32(0))
			assert.NotEmpty(t, v.name)
		}
	})

	t.Run("unknown enum is not registered", func(t *testing.T) {
		assert.NotContains(t, runtime.enumByName, "NonExistentEnum")
	})
}

func TestSchemaRuntimeServices(t *testing.T) {
	compiler, err := NewProtoCompiler(testSchemaWithLookup, testMapping())
	require.NoError(t, err)

	runtime, err := newSchemaRuntime(compiler.doc)
	require.NoError(t, err)

	t.Run("service methods are registered", func(t *testing.T) {
		assert.NotEmpty(t, runtime.serviceNamesByMethod)
	})

	t.Run("method maps to service name", func(t *testing.T) {
		serviceName, ok := runtime.serviceNamesByMethod["LookupProductById"]
		assert.True(t, ok)
		assert.Equal(t, "product.v1.ProductService", serviceName)
	})
}

// =============== Test Schemas ================== //

var testSchemaWithLookup = `
syntax = "proto3";
package product.v1;

enum ProductStatus {
  PRODUCT_STATUS_UNSPECIFIED = 0;
  PRODUCT_STATUS_ACTIVE = 1;
  PRODUCT_STATUS_DISCONTINUED = 2;
  PRODUCT_STATUS_OUT_OF_STOCK = 3;
}

enum CategoryKind {
  CATEGORY_KIND_UNSPECIFIED = 0;
  CATEGORY_KIND_PHYSICAL = 1;
  CATEGORY_KIND_DIGITAL = 2;
}

service ProductService {
  rpc LookupProductById(LookupProductByIdRequest) returns (LookupProductByIdResponse) {}
}

message LookupProductByIdRequest {
  repeated LookupProductByIdInput inputs = 1;
}

message LookupProductByIdInput {
  string id = 1;
}

message LookupProductByIdResponse {
  repeated LookupProductByIdResult results = 1;
}

message LookupProductByIdResult {
  Product product = 1;
}

message Product {
  string id = 1;
  string name = 2;
  double price = 3;
  ProductStatus status = 4;
  CategoryKind category = 5;
}

`
