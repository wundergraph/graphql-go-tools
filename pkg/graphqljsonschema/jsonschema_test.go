package graphqljsonschema

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
)

func prettyPrint(s string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	pretty, err := json.MarshalIndent(v, "  ", "     ")
	if err != nil {
		panic(err)
	}
	return string(pretty)
}

func runTest(schema, operation, expectedJsonSchema string, valid []string, invalid []string, opts ...Option) func(t *testing.T) {
	return func(t *testing.T) {
		definition := unsafeparser.ParseGraphqlDocumentString(schema)
		operationDoc := unsafeparser.ParseGraphqlDocumentString(operation)

		variableDefinition := operationDoc.OperationDefinitions[0].VariableDefinitions.Refs[0]
		varType := operationDoc.VariableDefinitions[variableDefinition].Type

		jsonSchemaDefinition := FromTypeRef(&operationDoc, &definition, varType, opts...)
		actualSchema, err := json.Marshal(jsonSchemaDefinition)
		assert.NoError(t, err)
		assert.Equal(t, prettyPrint(expectedJsonSchema), prettyPrint(string(actualSchema)))

		validator, err := NewValidatorFromString(string(actualSchema))
		assert.NoError(t, err)

		for _, input := range valid {
			assert.NoError(t, validator.Validate(context.Background(), []byte(input)), "Incorrectly judged invalid: %v", input)
		}

		for _, input := range invalid {
			assert.Error(t, validator.Validate(context.Background(), []byte(input)), "Incorrectly judged valid: %v", input)
		}
	}
}

func TestJsonSchema(t *testing.T) {
	t.Run("object", runTest(
		`scalar String input Test { str: String }`,
		`query ($input: Test){}`,
		`{"type":["object","null"],"properties":{"str":{"type":["string","null"]}},"additionalProperties":false}`,
		[]string{
			`{"str":"validString"}`,
			`{"str":null}`,
		},
		[]string{
			`{"str":true}`,
		},
	))
	t.Run("string", runTest(
		`scalar String input Test { str: String }`,
		`query ($input: String){}`,
		`{"type":["string","null"]}`,
		[]string{
			`"validString"`,
			`null`,
		},
		[]string{
			`false`,
			`true`,
			`nope`,
		},
	))
	t.Run("string (required)", runTest(
		`scalar String type Query { rootField(str: String!): String! }`,
		`query ($input: String!){ rootField(str: $input) }`,
		`{"type":["string"]}`,
		[]string{
			`"validString"`,
		},
		[]string{
			`false`,
			`true`,
			`nope`,
			`null`,
		},
	))
	t.Run("id", runTest(
		`scalar ID input Test { str: String }`,
		`query ($input: ID){}`,
		`{"type":["string","integer","null"]}`,
		[]string{
			`"validString"`,
			`null`,
		},
		[]string{
			`false`,
			`true`,
			`nope`,
		},
	))
	t.Run("array", runTest(
		`scalar String`,
		`query ($input: [String]){}`,
		`{"type":["array","null"],"items":{"type":["string","null"]}}`,
		[]string{
			`null`,
			`[]`,
			`["validString1"]`,
			`["validString1", "validString2"]`,
			`["validString1", "validString2", null]`,
		},
		[]string{
			`"validString"`,
			`false`,
		},
	))
	t.Run("input object array", runTest(
		`scalar String input StringInput { str: String }`,
		`query ($input: [StringInput]){}`,
		`{"type":["array","null"],"items":{"$ref":"#/$defs/StringInput"},"$defs":{"StringInput":{"type":["object","null"],"properties":{"str":{"type":["string","null"]}},"additionalProperties":false}}}`,
		[]string{
			`null`,
			`[]`,
			`[{"str":"validString1"}]`,
			`[{"str":"validString1"}, {"str":"validString2"}]`,
			`[{"str":"validString1"}, {"str":"validString2"}, null]`,
		},
		[]string{
			`"validString"`,
			`false`,
		},
	))
	t.Run("required array", runTest(
		`scalar String`,
		`query ($input: [String]!){}`,
		`{"type":["array"],"items":{"type":["string","null"]}}`,
		[]string{
			`[]`,
			`["validString1"]`,
			`["validString1", "validString2"]`,
			`["validString1", "validString2", null]`,
		},
		[]string{
			`"validString"`,
			`false`,
			`null`,
		},
	))
	t.Run("required array element", runTest(
		`scalar String`,
		`query ($input: [String!]){}`,
		`{"type":["array","null"],"items":{"type":["string"]}}`,
		[]string{
			`null`,
			`[]`,
			`["validString1"]`,
			`["validString1", "validString2"]`,
		},
		[]string{
			`[null]`,
			`["validString1", "validString2", null]`,
			`"validString"`,
			`false`,
		},
	))
	t.Run("nested object", runTest(
		`scalar String scalar Boolean input Test { str: String! nested: Nested } input Nested { boo: Boolean }`,
		`query ($input: Test){}`,
		`{"type":["object","null"],"properties":{"nested":{"$ref":"#/$defs/Nested"},"str":{"type":["string"]}},"required":["str"],"additionalProperties":false,"$defs":{"Nested":{"type":["object","null"],"properties":{"boo":{"type":["boolean","null"]}},"additionalProperties":false}}}`,
		[]string{
			`null`,
			`{"str":"validString"}`,
			`{"str":"validString","nested":null}`,
			`{"str":"validString","nested":{"boo":true}}`,
			`{"str":"validString","nested":{"boo":null}}`,
			`{"str":"validString","nested":{}}`,
		},
		[]string{
			`{"str":true}`,
			`{"str":null}`,
			`{"nested":{"boo":true}}`,
			`{"str":"validString","nested":{"boo":123}}`,
		},
	))
	t.Run("nested object with override", runTest(
		`scalar String scalar Boolean input Test { str: String! override: Override } input Override { boo: Boolean }`,
		`query ($input: Test){}`,
		`{"type":["object","null"],"properties":{"override":{"type":["string","null"]},"str":{"type":["string"]}},"required":["str"],"additionalProperties":false}`,
		[]string{
			`null`,
			`{"str":"validString"}`,
			`{"str":"validString","override":"{\"boo\":true}"}`,
			`{"str":"validString","override":null}`,
		},
		[]string{
			`{"str":true}`,
			`{"str":null}`,
			`{"override":{"boo":true}}`,
			`{"str":"validString","override":{"boo":123}}`,
		},
		WithOverrides(map[string]JsonSchema{
			"Override": NewString(false),
		}),
	))
	t.Run("recursive object", runTest(
		`scalar String scalar Boolean input Test { str: String! nested: Nested } input Nested { boo: Boolean recursive: Test }`,
		`query ($input: Test){}`,
		`{"type":["object","null"],"properties":{"nested":{"$ref":"#/$defs/Nested"},"str":{"type":["string"]}},"required":["str"],"additionalProperties":false,"$defs":{"Nested":{"type":["object","null"],"properties":{"boo":{"type":["boolean","null"]},"recursive":{"$ref":"#/$defs/Test"}},"additionalProperties":false},"Test":{"type":["object","null"],"properties":{"nested":{"$ref":"#/$defs/Nested"},"str":{"type":["string"]}},"required":["str"],"additionalProperties":false}}}`,
		[]string{
			`{"str":"validString"}`,
			`{"str":"validString","nested":{"boo":true}}`,
			`{"str":"validString","nested":{"boo":null}}`,
			`{"str":"validString","nested":null}`,
		},
		[]string{
			`{"str":true}`,
			`{"nested":{"boo":true}}`,
			`{"str":"validString","nested":{"boo":123}}`,
		},
	))
	t.Run("recursive object with multiple branches", runTest(
		`scalar String scalar Boolean input Root { test: Test another: Another } input Test { str: String! nested: Nested } input Nested { boo: Boolean recursive: Test another: Another } input Another { boo: Boolean }`,
		`query ($input: Root){}`,
		`{"type":["object","null"],"properties":{"another":{"$ref":"#/$defs/Another"},"test":{"$ref":"#/$defs/Test"}},"additionalProperties":false,"$defs":{"Another":{"type":["object","null"],"properties":{"boo":{"type":["boolean","null"]}},"additionalProperties":false},"Nested":{"type":["object","null"],"properties":{"another":{"$ref":"#/$defs/Another"},"boo":{"type":["boolean","null"]},"recursive":{"$ref":"#/$defs/Test"}},"additionalProperties":false},"Test":{"type":["object","null"],"properties":{"nested":{"$ref":"#/$defs/Nested"},"str":{"type":["string"]}},"required":["str"],"additionalProperties":false}}}`,
		[]string{
			`{"test":{"str":"validString"}}`,
			`{"test":{"str":"validString","nested":{"boo":true}}}`,
		},
		[]string{
			`{"test":{"str":true}}`,
			`{"test":{"nested":{"boo":true}}}`,
			`{"test":{"str":"validString","nested":{"boo":123}}}`,
		},
	))
	t.Run("complex recursive schema", runTest(
		complexRecursiveSchema,
		`query ($input: db_messagesWhereInput){}`,
		complexRecursiveSchemaResult,
		[]string{},
		[]string{},
	))
	t.Run("one level deep sub path", runTest(
		"input Human { name: String! } scalar String",
		"query ($human: Human!) { }",
		`{"type":["string"]}`,
		[]string{
			`"John Doe"`,
		},
		[]string{
			`{"name":"John Doe"}`,
		},
		WithPath([]string{"name"}),
	))
	t.Run("multi level deep sub path", runTest(
		"input Human { name: String! pet: Animal } scalar String type Animal { name: String! }",
		"query ($human: Human!) { }",
		`{"type":["string"]}`,
		[]string{
			`"Doggie"`,
		},
		[]string{
			`{"name":"Doggie"}`,
			`{"pet":{"name":"Doggie"}}`,
		},
		WithPath([]string{"pet", "name"}),
	))
	t.Run("not defined scalar", runTest(
		`input Container { name: MyScalar }`,
		`query ($input: Container){}`,
		`{"type":["object", "null"], "properties": {"name": {}}, "additionalProperties": false}`,
		[]string{},
		[]string{},
	))
}

const complexRecursiveSchema = `
scalar Int scalar String

input db_NestedIntFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntFilter
}

input db_IntFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntFilter
}

enum db_QueryMode {
  default
  insensitive
}

input db_NestedStringFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: db_NestedStringFilter
}

input db_StringFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: db_QueryMode
  not: db_NestedStringFilter
}

enum db_JsonNullValueFilter {
  DbNull
  JsonNull
  AnyNull
}

input db_JsonFilter {
  equals: db_JsonNullValueFilter
  not: db_JsonNullValueFilter
}

input db_NestedDateTimeFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeFilter
}

input db_DateTimeFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeFilter
}

input db_MessagesListRelationFilter {
  every: db_messagesWhereInput
  some: db_messagesWhereInput
  none: db_messagesWhereInput
}

input db_usersWhereInput {
  AND: db_usersWhereInput
  OR: [db_usersWhereInput]
  NOT: db_usersWhereInput
  id: db_IntFilter
  email: db_StringFilter
  name: db_StringFilter
  updatedat: db_DateTimeFilter
  lastlogin: db_DateTimeFilter
  pet: db_StringFilter
  messages: db_MessagesListRelationFilter
}

input db_UsersRelationFilter {
  is: db_usersWhereInput
  isNot: db_usersWhereInput
}

input db_messagesWhereInput {
  AND: db_messagesWhereInput
  OR: [db_messagesWhereInput]
  NOT: db_messagesWhereInput
  id: db_IntFilter
  user_id: db_IntFilter
  message: db_StringFilter
  payload: db_JsonFilter
  users: db_UsersRelationFilter
}

enum db_SortOrder {
  asc
  desc
}

input db_messagesOrderByRelationAggregateInput {
  _count: db_SortOrder
}

input db_usersOrderByWithRelationInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
  messages: db_messagesOrderByRelationAggregateInput
}

input db_messagesOrderByWithRelationInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
  payload: db_SortOrder
  users: db_usersOrderByWithRelationInput
}

input db_messagesWhereUniqueInput {
  id: Int
}

enum db_MessagesScalarFieldEnum {
  id
  user_id
  message
  payload
}

type db_UsersCountOutputType {
  messages: Int!
  _join: Query!
}

type db_users {
  id: Int!
  email: String!
  name: String!
  updatedat: DateTime!
  lastlogin: DateTime!
  pet: String!
  messages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int, distinct: [db_MessagesScalarFieldEnum]): [db_messages]
  _count: db_UsersCountOutputType
  _join: Query!
}

type db_messages {
  id: Int!
  user_id: Int!
  message: String!
  payload: db_Widgets!
  users: db_users!
  _join: Query!
}

type db_MessagesCountAggregateOutputType {
  id: Int!
  user_id: Int!
  message: Int!
  payload: Int!
  _all: Int!
  _join: Query!
}

type db_MessagesAvgAggregateOutputType {
  id: Float
  user_id: Float
  _join: Query!
}

type db_MessagesSumAggregateOutputType {
  id: Int
  user_id: Int
  _join: Query!
}

type db_MessagesMinAggregateOutputType {
  id: Int
  user_id: Int
  message: String
  _join: Query!
}

type db_MessagesMaxAggregateOutputType {
  id: Int
  user_id: Int
  message: String
  _join: Query!
}

type db_AggregateMessages {
  _count: db_MessagesCountAggregateOutputType
  _avg: db_MessagesAvgAggregateOutputType
  _sum: db_MessagesSumAggregateOutputType
  _min: db_MessagesMinAggregateOutputType
  _max: db_MessagesMaxAggregateOutputType
  _join: Query!
}

input db_messagesCountOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
  payload: db_SortOrder
}

input db_messagesAvgOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
}

input db_messagesMaxOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
}

input db_messagesMinOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
}

input db_messagesSumOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
}

input db_messagesOrderByWithAggregationInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
  payload: db_SortOrder
  _count: db_messagesCountOrderByAggregateInput
  _avg: db_messagesAvgOrderByAggregateInput
  _max: db_messagesMaxOrderByAggregateInput
  _min: db_messagesMinOrderByAggregateInput
  _sum: db_messagesSumOrderByAggregateInput
}

input db_NestedFloatFilter {
  equals: Float
  in: [Float]
  notIn: [Float]
  lt: Float
  lte: Float
  gt: Float
  gte: Float
  not: db_NestedFloatFilter
}

input db_NestedIntWithAggregatesFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntWithAggregatesFilter
  _count: db_NestedIntFilter
  _avg: db_NestedFloatFilter
  _sum: db_NestedIntFilter
  _min: db_NestedIntFilter
  _max: db_NestedIntFilter
}

input db_IntWithAggregatesFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntWithAggregatesFilter
  _count: db_NestedIntFilter
  _avg: db_NestedFloatFilter
  _sum: db_NestedIntFilter
  _min: db_NestedIntFilter
  _max: db_NestedIntFilter
}

input db_NestedStringWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: db_NestedStringWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedStringFilter
  _max: db_NestedStringFilter
}

input db_StringWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: db_QueryMode
  not: db_NestedStringWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedStringFilter
  _max: db_NestedStringFilter
}

input db_NestedJsonFilter {
  equals: db_JsonNullValueFilter
  not: db_JsonNullValueFilter
}

input db_JsonWithAggregatesFilter {
  equals: db_JsonNullValueFilter
  not: db_JsonNullValueFilter
  _count: db_NestedIntFilter
  _min: db_NestedJsonFilter
  _max: db_NestedJsonFilter
}

input db_messagesScalarWhereWithAggregatesInput {
  AND: db_messagesScalarWhereWithAggregatesInput
  OR: [db_messagesScalarWhereWithAggregatesInput]
  NOT: db_messagesScalarWhereWithAggregatesInput
  id: db_IntWithAggregatesFilter
  user_id: db_IntWithAggregatesFilter
  message: db_StringWithAggregatesFilter
  payload: db_JsonWithAggregatesFilter
}

type db_MessagesGroupByOutputType {
  id: Int!
  user_id: Int!
  message: String!
  payload: JSON!
  _count: db_MessagesCountAggregateOutputType
  _avg: db_MessagesAvgAggregateOutputType
  _sum: db_MessagesSumAggregateOutputType
  _min: db_MessagesMinAggregateOutputType
  _max: db_MessagesMaxAggregateOutputType
  _join: Query!
}

input db_usersWhereUniqueInput {
  id: Int
  email: String
}

enum db_UsersScalarFieldEnum {
  id
  email
  name
  updatedat
  lastlogin
  pet
}

type db_UsersCountAggregateOutputType {
  id: Int!
  email: Int!
  name: Int!
  updatedat: Int!
  lastlogin: Int!
  pet: Int!
  _all: Int!
  _join: Query!
}

type db_UsersAvgAggregateOutputType {
  id: Float
  _join: Query!
}

type db_UsersSumAggregateOutputType {
  id: Int
  _join: Query!
}

type db_UsersMinAggregateOutputType {
  id: Int
  email: String
  name: String
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
  _join: Query!
}

type db_UsersMaxAggregateOutputType {
  id: Int
  email: String
  name: String
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
  _join: Query!
}

type db_AggregateUsers {
  _count: db_UsersCountAggregateOutputType
  _avg: db_UsersAvgAggregateOutputType
  _sum: db_UsersSumAggregateOutputType
  _min: db_UsersMinAggregateOutputType
  _max: db_UsersMaxAggregateOutputType
  _join: Query!
}

input db_usersCountOrderByAggregateInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
}

input db_usersAvgOrderByAggregateInput {
  id: db_SortOrder
}

input db_usersMaxOrderByAggregateInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
}

input db_usersMinOrderByAggregateInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
}

input db_usersSumOrderByAggregateInput {
  id: db_SortOrder
}

input db_usersOrderByWithAggregationInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
  _count: db_usersCountOrderByAggregateInput
  _avg: db_usersAvgOrderByAggregateInput
  _max: db_usersMaxOrderByAggregateInput
  _min: db_usersMinOrderByAggregateInput
  _sum: db_usersSumOrderByAggregateInput
}

input db_NestedDateTimeWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedDateTimeFilter
  _max: db_NestedDateTimeFilter
}

input db_DateTimeWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedDateTimeFilter
  _max: db_NestedDateTimeFilter
}

input db_usersScalarWhereWithAggregatesInput {
  AND: db_usersScalarWhereWithAggregatesInput
  OR: [db_usersScalarWhereWithAggregatesInput]
  NOT: db_usersScalarWhereWithAggregatesInput
  id: db_IntWithAggregatesFilter
  email: db_StringWithAggregatesFilter
  name: db_StringWithAggregatesFilter
  updatedat: db_DateTimeWithAggregatesFilter
  lastlogin: db_DateTimeWithAggregatesFilter
  pet: db_StringWithAggregatesFilter
}

type db_UsersGroupByOutputType {
  id: Int!
  email: String!
  name: String!
  updatedat: DateTime!
  lastlogin: DateTime!
  pet: String!
  _count: db_UsersCountAggregateOutputType
  _avg: db_UsersAvgAggregateOutputType
  _sum: db_UsersSumAggregateOutputType
  _min: db_UsersMinAggregateOutputType
  _max: db_UsersMaxAggregateOutputType
  _join: Query!
}

type Query {
  db_findFirstmessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int, distinct: [db_MessagesScalarFieldEnum]): db_messages
  db_findManymessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int, distinct: [db_MessagesScalarFieldEnum]): [db_messages]!
  db_aggregatemessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int): db_AggregateMessages!
  db_groupBymessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithAggregationInput], by: [db_MessagesScalarFieldEnum]!, having: db_messagesScalarWhereWithAggregatesInput, take: Int, skip: Int): [db_MessagesGroupByOutputType]!
  db_findUniquemessages(where: db_messagesWhereUniqueInput!): db_messages
  db_findFirstusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithRelationInput], cursor: db_usersWhereUniqueInput, take: Int, skip: Int, distinct: [db_UsersScalarFieldEnum]): db_users
  db_findManyusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithRelationInput], cursor: db_usersWhereUniqueInput, take: Int, skip: Int, distinct: [db_UsersScalarFieldEnum]): [db_users]!
  db_aggregateusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithRelationInput], cursor: db_usersWhereUniqueInput, take: Int, skip: Int): db_AggregateUsers!
  db_groupByusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithAggregationInput], by: [db_UsersScalarFieldEnum]!, having: db_usersScalarWhereWithAggregatesInput, take: Int, skip: Int): [db_UsersGroupByOutputType]!
  db_findUniqueusers(where: db_usersWhereUniqueInput!): db_users
}

input db_usersCreateWithoutMessagesInput {
  email: String!
  name: String!
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
}

input db_usersCreateOrConnectWithoutMessagesInput {
  where: db_usersWhereUniqueInput!
  create: db_usersCreateWithoutMessagesInput!
}

input db_usersCreateNestedOneWithoutMessagesInput {
  create: db_usersCreateWithoutMessagesInput
  connectOrCreate: db_usersCreateOrConnectWithoutMessagesInput
  connect: db_usersWhereUniqueInput
}

input db_messagesCreateInput {
  message: String!
  payload: db_WidgetsInput
  users: db_usersCreateNestedOneWithoutMessagesInput!
}

input db_StringFieldUpdateOperationsInput {
  set: String
}

input db_DateTimeFieldUpdateOperationsInput {
  set: DateTime
}

input db_usersUpdateWithoutMessagesInput {
  email: db_StringFieldUpdateOperationsInput
  name: db_StringFieldUpdateOperationsInput
  updatedat: db_DateTimeFieldUpdateOperationsInput
  lastlogin: db_DateTimeFieldUpdateOperationsInput
  pet: db_StringFieldUpdateOperationsInput
}

input db_usersUpsertWithoutMessagesInput {
  update: db_usersUpdateWithoutMessagesInput!
  create: db_usersCreateWithoutMessagesInput!
}

input db_usersUpdateOneRequiredWithoutMessagesInput {
  create: db_usersCreateWithoutMessagesInput
  connectOrCreate: db_usersCreateOrConnectWithoutMessagesInput
  upsert: db_usersUpsertWithoutMessagesInput
  connect: db_usersWhereUniqueInput
  update: db_usersUpdateWithoutMessagesInput
}

input db_messagesUpdateInput {
  message: db_StringFieldUpdateOperationsInput
  payload: db_WidgetsInput
  users: db_usersUpdateOneRequiredWithoutMessagesInput
}

input db_messagesCreateManyInput {
  id: Int
  user_id: Int!
  message: String!
  payload: db_WidgetsInput
}

type db_AffectedRowsOutput {
  count: Int!
  _join: Query!
}

input db_messagesUpdateManyMutationInput {
  message: db_StringFieldUpdateOperationsInput
  payload: db_WidgetsInput
}

input db_messagesCreateWithoutUsersInput {
  message: String!
  payload: db_WidgetsInput
}

input db_messagesCreateOrConnectWithoutUsersInput {
  where: db_messagesWhereUniqueInput!
  create: db_messagesCreateWithoutUsersInput!
}

input db_messagesCreateManyUsersInput {
  id: Int
  message: String!
  payload: db_WidgetsInput
}

input db_messagesCreateManyUsersInputEnvelope {
  data: [db_messagesCreateManyUsersInput]!
  skipDuplicates: Boolean
}

input db_messagesCreateNestedManyWithoutUsersInput {
  create: db_messagesCreateWithoutUsersInput
  connectOrCreate: db_messagesCreateOrConnectWithoutUsersInput
  createMany: db_messagesCreateManyUsersInputEnvelope
  connect: db_messagesWhereUniqueInput
}

input db_usersCreateInput {
  email: String!
  name: String!
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
  messages: db_messagesCreateNestedManyWithoutUsersInput
}

input db_messagesUpdateWithoutUsersInput {
  message: db_StringFieldUpdateOperationsInput
  payload: db_WidgetsInput
}

input db_messagesUpsertWithWhereUniqueWithoutUsersInput {
  where: db_messagesWhereUniqueInput!
  update: db_messagesUpdateWithoutUsersInput!
  create: db_messagesCreateWithoutUsersInput!
}

input db_messagesUpdateWithWhereUniqueWithoutUsersInput {
  where: db_messagesWhereUniqueInput!
  data: db_messagesUpdateWithoutUsersInput!
}

input db_messagesScalarWhereInput {
  AND: db_messagesScalarWhereInput
  OR: [db_messagesScalarWhereInput]
  NOT: db_messagesScalarWhereInput
  id: db_IntFilter
  user_id: db_IntFilter
  message: db_StringFilter
  payload: db_JsonFilter
}

input db_messagesUpdateManyWithWhereWithoutUsersInput {
  where: db_messagesScalarWhereInput!
  data: db_messagesUpdateManyMutationInput!
}

input db_messagesUpdateManyWithoutUsersInput {
  create: db_messagesCreateWithoutUsersInput
  connectOrCreate: db_messagesCreateOrConnectWithoutUsersInput
  upsert: db_messagesUpsertWithWhereUniqueWithoutUsersInput
  createMany: db_messagesCreateManyUsersInputEnvelope
  connect: db_messagesWhereUniqueInput
  set: db_messagesWhereUniqueInput
  disconnect: db_messagesWhereUniqueInput
  delete: db_messagesWhereUniqueInput
  update: db_messagesUpdateWithWhereUniqueWithoutUsersInput
  updateMany: db_messagesUpdateManyWithWhereWithoutUsersInput
  deleteMany: db_messagesScalarWhereInput
}

input db_usersUpdateInput {
  email: db_StringFieldUpdateOperationsInput
  name: db_StringFieldUpdateOperationsInput
  updatedat: db_DateTimeFieldUpdateOperationsInput
  lastlogin: db_DateTimeFieldUpdateOperationsInput
  pet: db_StringFieldUpdateOperationsInput
  messages: db_messagesUpdateManyWithoutUsersInput
}

input db_usersCreateManyInput {
  id: Int
  email: String!
  name: String!
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
}

input db_usersUpdateManyMutationInput {
  email: db_StringFieldUpdateOperationsInput
  name: db_StringFieldUpdateOperationsInput
  updatedat: db_DateTimeFieldUpdateOperationsInput
  lastlogin: db_DateTimeFieldUpdateOperationsInput
  pet: db_StringFieldUpdateOperationsInput
}

type Mutation {
  db_createOnemessages(data: db_messagesCreateInput!): db_messages
  db_upsertOnemessages(where: db_messagesWhereUniqueInput!, create: db_messagesCreateInput!, update: db_messagesUpdateInput!): db_messages
  db_createManymessages(data: [db_messagesCreateManyInput]!, skipDuplicates: Boolean): db_AffectedRowsOutput
  db_deleteOnemessages(where: db_messagesWhereUniqueInput!): db_messages
  db_updateOnemessages(data: db_messagesUpdateInput!, where: db_messagesWhereUniqueInput!): db_messages
  db_updateManymessages(data: db_messagesUpdateManyMutationInput!, where: db_messagesWhereInput): db_AffectedRowsOutput
  db_deleteManymessages(where: db_messagesWhereInput): db_AffectedRowsOutput
  db_createOneusers(data: db_usersCreateInput!): db_users
  db_upsertOneusers(where: db_usersWhereUniqueInput!, create: db_usersCreateInput!, update: db_usersUpdateInput!): db_users
  db_createManyusers(data: [db_usersCreateManyInput]!, skipDuplicates: Boolean): db_AffectedRowsOutput
  db_deleteOneusers(where: db_usersWhereUniqueInput!): db_users
  db_updateOneusers(data: db_usersUpdateInput!, where: db_usersWhereUniqueInput!): db_users
  db_updateManyusers(data: db_usersUpdateManyMutationInput!, where: db_usersWhereInput): db_AffectedRowsOutput
  db_deleteManyusers(where: db_usersWhereInput): db_AffectedRowsOutput
}

scalar DateTime

scalar JSON

scalar UUID

type db_Widget {
  id: ID!
  type: String!
  name: String
  options: JSON
  x: Int!
  y: Int!
  width: Int!
  height: Int!
  _join: Query!
}

type db_Widgets {
  items: [db_Widget]!
  _join: Query!
}

input db_WidgetInput {
  id: ID!
  type: String!
  name: String
  options: JSON
  x: Int!
  y: Int!
  width: Int!
  height: Int!
}

input db_WidgetsInput {
  items: [db_WidgetInput]!
}
`

const complexRecursiveSchemaResult = `
{
  "type": [
      "object",
      "null"
  ],
  "properties": {
      "AND": {
          "$ref": "#/$defs/db_messagesWhereInput"
      },
      "NOT": {
          "$ref": "#/$defs/db_messagesWhereInput"
      },
      "OR": {
          "type": [
              "array",
              "null"
          ],
          "items": {
              "$ref": "#/$defs/db_messagesWhereInput"
          }
      },
      "id": {
          "$ref": "#/$defs/db_IntFilter"
      },
      "message": {
          "$ref": "#/$defs/db_StringFilter"
      },
      "payload": {
          "$ref": "#/$defs/db_JsonFilter"
      },
      "user_id": {
          "$ref": "#/$defs/db_IntFilter"
      },
      "users": {
          "$ref": "#/$defs/db_UsersRelationFilter"
      }
  },
  "additionalProperties": false,
  "$defs": {
      "db_DateTimeFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "equals": {},
              "gt": {},
              "gte": {},
              "in": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {}
              },
              "lt": {},
              "lte": {},
              "not": {
                  "$ref": "#/$defs/db_NestedDateTimeFilter"
              },
              "notIn": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {}
              }
          },
          "additionalProperties": false
      },
      "db_IntFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "equals": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "gt": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "gte": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "in": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "integer",
                          "null"
                      ]
                  }
              },
              "lt": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "lte": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "not": {
                  "$ref": "#/$defs/db_NestedIntFilter"
              },
              "notIn": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "integer",
                          "null"
                      ]
                  }
              }
          },
          "additionalProperties": false
      },
      "db_JsonFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "equals": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "not": {
                  "type": [
                      "string",
                      "null"
                  ]
              }
          },
          "additionalProperties": false
      },
      "db_MessagesListRelationFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "every": {
                  "$ref": "#/$defs/db_messagesWhereInput"
              },
              "none": {
                  "$ref": "#/$defs/db_messagesWhereInput"
              },
              "some": {
                  "$ref": "#/$defs/db_messagesWhereInput"
              }
          },
          "additionalProperties": false
      },
      "db_NestedDateTimeFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "equals": {},
              "gt": {},
              "gte": {},
              "in": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {}
              },
              "lt": {},
              "lte": {},
              "not": {
                  "$ref": "#/$defs/db_NestedDateTimeFilter"
              },
              "notIn": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {}
              }
          },
          "additionalProperties": false
      },
      "db_NestedIntFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "equals": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "gt": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "gte": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "in": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "integer",
                          "null"
                      ]
                  }
              },
              "lt": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "lte": {
                  "type": [
                      "integer",
                      "null"
                  ]
              },
              "not": {
                  "$ref": "#/$defs/db_NestedIntFilter"
              },
              "notIn": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "integer",
                          "null"
                      ]
                  }
              }
          },
          "additionalProperties": false
      },
      "db_NestedStringFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "contains": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "endsWith": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "equals": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "gt": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "gte": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "in": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "string",
                          "null"
                      ]
                  }
              },
              "lt": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "lte": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "not": {
                  "$ref": "#/$defs/db_NestedStringFilter"
              },
              "notIn": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "string",
                          "null"
                      ]
                  }
              },
              "startsWith": {
                  "type": [
                      "string",
                      "null"
                  ]
              }
          },
          "additionalProperties": false
      },
      "db_StringFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "contains": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "endsWith": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "equals": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "gt": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "gte": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "in": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "string",
                          "null"
                      ]
                  }
              },
              "lt": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "lte": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "mode": {
                  "type": [
                      "string",
                      "null"
                  ]
              },
              "not": {
                  "$ref": "#/$defs/db_NestedStringFilter"
              },
              "notIn": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "type": [
                          "string",
                          "null"
                      ]
                  }
              },
              "startsWith": {
                  "type": [
                      "string",
                      "null"
                  ]
              }
          },
          "additionalProperties": false
      },
      "db_UsersRelationFilter": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "is": {
                  "$ref": "#/$defs/db_usersWhereInput"
              },
              "isNot": {
                  "$ref": "#/$defs/db_usersWhereInput"
              }
          },
          "additionalProperties": false
      },
      "db_messagesWhereInput": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "AND": {
                  "$ref": "#/$defs/db_messagesWhereInput"
              },
              "NOT": {
                  "$ref": "#/$defs/db_messagesWhereInput"
              },
              "OR": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "$ref": "#/$defs/db_messagesWhereInput"
                  }
              },
              "id": {
                  "$ref": "#/$defs/db_IntFilter"
              },
              "message": {
                  "$ref": "#/$defs/db_StringFilter"
              },
              "payload": {
                  "$ref": "#/$defs/db_JsonFilter"
              },
              "user_id": {
                  "$ref": "#/$defs/db_IntFilter"
              },
              "users": {
                  "$ref": "#/$defs/db_UsersRelationFilter"
              }
          },
          "additionalProperties": false
      },
      "db_usersWhereInput": {
          "type": [
              "object",
              "null"
          ],
          "properties": {
              "AND": {
                  "$ref": "#/$defs/db_usersWhereInput"
              },
              "NOT": {
                  "$ref": "#/$defs/db_usersWhereInput"
              },
              "OR": {
                  "type": [
                      "array",
                      "null"
                  ],
                  "items": {
                      "$ref": "#/$defs/db_usersWhereInput"
                  }
              },
              "email": {
                  "$ref": "#/$defs/db_StringFilter"
              },
              "id": {
                  "$ref": "#/$defs/db_IntFilter"
              },
              "lastlogin": {
                  "$ref": "#/$defs/db_DateTimeFilter"
              },
              "messages": {
                  "$ref": "#/$defs/db_MessagesListRelationFilter"
              },
              "name": {
                  "$ref": "#/$defs/db_StringFilter"
              },
              "pet": {
                  "$ref": "#/$defs/db_StringFilter"
              },
              "updatedat": {
                  "$ref": "#/$defs/db_DateTimeFilter"
              }
          },
          "additionalProperties": false
      }
  }
}
`
