package grpcdatasource

import (
	"testing"
)

func TestNullableFieldsExecutionPlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with nullable fields type",
			query: "query NullableFieldsTypeQuery { nullableFieldsType { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsType",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:          "nullable_fields_type",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "nullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "optional_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "optionalInt",
												Optional:      true,
											},
											{
												Name:          "optional_float",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "optionalFloat",
												Optional:      true,
											},
											{
												Name:          "optional_boolean",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "optionalBoolean",
												Optional:      true,
											},
											{
												Name:          "required_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "requiredString",
											},
											{
												Name:          "required_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "requiredInt",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Should create an execution plan for a query with nullable fields in the request",
			query: `query NullableFieldsTypeWithFilterQuery($filter: NullableFieldsFilter!) { nullableFieldsTypeWithFilter(filter: $filter) { id name optionalString optionalInt optionalFloat optionalBoolean } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsTypeWithFilter",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeWithFilterRequest",
							Fields: []RPCField{
								{
									Name:          "filter",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "filter",
									Message: &RPCMessage{
										Name: "NullableFieldsFilter",
										Fields: []RPCField{
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												Optional:      true,
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "include_nulls",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "includeNulls",
												Optional:      true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeWithFilterResponse",
							Fields: []RPCField{
								{
									Name:          "nullable_fields_type_with_filter",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "nullableFieldsTypeWithFilter",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "optional_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "optionalInt",
												Optional:      true,
											},
											{
												Name:          "optional_float",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "optionalFloat",
												Optional:      true,
											},
											{
												Name:          "optional_boolean",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "optionalBoolean",
												Optional:      true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Should create an execution plan for nullable fields type by ID query",
			query: `query NullableFieldsTypeByIdQuery($id: ID!) { nullableFieldsTypeById(id: $id) { id name optionalString requiredString } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsTypeById",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeByIdRequest",
							Fields: []RPCField{
								{
									Name:          "id",
									ProtoTypeName: DataTypeString,
									JSONPath:      "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeByIdResponse",
							Fields: []RPCField{
								{
									Name:          "nullable_fields_type_by_id",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "nullableFieldsTypeById",
									Optional:      true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "required_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "requiredString",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Should create an execution plan for all nullable fields types query",
			query: "query AllNullableFieldsTypesQuery { allNullableFieldsTypes { id name optionalString optionalInt requiredString requiredInt } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryAllNullableFieldsTypes",
						Request: RPCMessage{
							Name: "QueryAllNullableFieldsTypesRequest",
						},
						Response: RPCMessage{
							Name: "QueryAllNullableFieldsTypesResponse",
							Fields: []RPCField{
								{
									Name:          "all_nullable_fields_types",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "allNullableFieldsTypes",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "optional_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "optionalInt",
												Optional:      true,
											},
											{
												Name:          "required_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "requiredString",
											},
											{
												Name:          "required_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "requiredInt",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Should create an execution plan for create nullable fields type mutation",
			query: `mutation CreateNullableFieldsType($input: NullableFieldsInput!) { createNullableFieldsType(input: $input) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationCreateNullableFieldsType",
						Request: RPCMessage{
							Name: "MutationCreateNullableFieldsTypeRequest",
							Fields: []RPCField{
								{
									Name:          "input",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "input",
									Message: &RPCMessage{
										Name: "NullableFieldsInput",
										Fields: []RPCField{
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "optional_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "optionalInt",
												Optional:      true,
											},
											{
												Name:          "optional_float",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "optionalFloat",
												Optional:      true,
											},
											{
												Name:          "optional_boolean",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "optionalBoolean",
												Optional:      true,
											},
											{
												Name:          "required_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "requiredString",
											},
											{
												Name:          "required_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "requiredInt",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationCreateNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:          "create_nullable_fields_type",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "createNullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "optional_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "optionalInt",
												Optional:      true,
											},
											{
												Name:          "optional_float",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "optionalFloat",
												Optional:      true,
											},
											{
												Name:          "optional_boolean",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "optionalBoolean",
												Optional:      true,
											},
											{
												Name:          "required_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "requiredString",
											},
											{
												Name:          "required_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "requiredInt",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Should create an execution plan for update nullable fields type mutation",
			query: `mutation UpdateNullableFieldsType($id: ID!, $input: NullableFieldsInput!) { updateNullableFieldsType(id: $id, input: $input) { id name optionalString requiredString } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationUpdateNullableFieldsType",
						Request: RPCMessage{
							Name: "MutationUpdateNullableFieldsTypeRequest",
							Fields: []RPCField{
								{
									Name:          "id",
									ProtoTypeName: DataTypeString,
									JSONPath:      "id",
								},
								{
									Name:          "input",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "input",
									Message: &RPCMessage{
										Name: "NullableFieldsInput",
										Fields: []RPCField{
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "optional_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "optionalInt",
												Optional:      true,
											},
											{
												Name:          "optional_float",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "optionalFloat",
												Optional:      true,
											},
											{
												Name:          "optional_boolean",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "optionalBoolean",
												Optional:      true,
											},
											{
												Name:          "required_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "requiredString",
											},
											{
												Name:          "required_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "requiredInt",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationUpdateNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:          "update_nullable_fields_type",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "updateNullableFieldsType",
									Optional:      true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "required_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "requiredString",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Should create an execution plan for nullable fields with partial field selection",
			query: "query PartialNullableFieldsQuery { nullableFieldsType { id optionalString } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsType",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:          "nullable_fields_type",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "nullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Should create an execution plan for nullable fields with only optional fields",
			query: "query OptionalFieldsOnlyQuery { nullableFieldsType { optionalString optionalInt optionalFloat optionalBoolean } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsType",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:          "nullable_fields_type",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "nullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:          "optional_string",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalString",
												Optional:      true,
											},
											{
												Name:          "optional_int",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "optionalInt",
												Optional:      true,
											},
											{
												Name:          "optional_float",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "optionalFloat",
												Optional:      true,
											},
											{
												Name:          "optional_boolean",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "optionalBoolean",
												Optional:      true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runTest(t, testCase{
				query:         tt.query,
				expectedPlan:  tt.expectedPlan,
				expectedError: tt.expectedError,
			})
		})
	}
}
