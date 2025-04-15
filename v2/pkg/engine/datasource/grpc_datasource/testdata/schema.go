package testdata

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

var UpstreamSchema = `
	scalar ID
	scalar String
	scalar Float

type Product @key(fields: "id") {
	id: ID!
	name: String!
	price: Float!
	shippingEstimate(input: ShippingEstimateInput!): Float!
}

type Storage @key(fields: "id") {
	id: ID!
	name: String!
	location: String!
}

type User {
	id: ID!
	name: String!
}

type NestedTypeA {
	id: ID!
	name: String!
	b: NestedTypeB!
}

type NestedTypeB {
	id: ID!
	name: String!
	c: NestedTypeC!
}

type NestedTypeC {
	id: ID!
	name: String!
}

type RecursiveType {
	id: ID!
	name: String!
	recursiveType: RecursiveType!
}

type TypeWithMultipleFilterFields {
	id: ID!
	name: String!
	filterField1: String!
	filterField2: String!
}

input FilterTypeInput {
	filterField1: String!
	filterField2: String!
}

type TypeWithComplexFilterInput {
	id: ID!
	name: String!
}

input FilterType {
	name: String!
	filterField1: String!
	filterField2: String!
	pagination: Pagination!
}

input Pagination {
	page: Int!
	perPage: Int!
}

input ComplexFilterTypeInput {
	filter: FilterType!
}


type Query {
	_entities(representations: [_Any!]!): [_Entity!]!
	users: [User!]!
	user(id: ID!): User
	nestedType: [NestedTypeA!]!
	recursiveType: RecursiveType!
	typeFilterWithArguments(filterField1: String!, filterField2: String!): [TypeWithMultipleFilterFields!]!
	typeWithMultipleFilterFields(filter: FilterTypeInput!): [TypeWithMultipleFilterFields!]!
	complexFilterType(filter: ComplexFilterTypeInput!): [TypeWithComplexFilterInput!]!
}

union _Entity = Product | Storage
scalar _Any
`

var ProtoSchema = func(t *testing.T) string {
	// get current directory with runtime.Caller
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to get current directory")
	}

	dir := filepath.Dir(filename)

	content, err := os.ReadFile(filepath.Join(dir, "product.proto"))
	if err != nil {
		t.Fatalf("failed to read product.proto: %v", err)
	}

	require.NotEmpty(t, content, "product.proto is empty")
	return string(content)
}
