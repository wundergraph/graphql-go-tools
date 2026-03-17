package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const protoSchemaWithOptionalNestedInputs = `
syntax = "proto3";
package rule.v1;

import "google/protobuf/wrappers.proto";

service RuleService {
  rpc UpdateRule(UpdateRuleRequest) returns (UpdateRuleResponse) {}
}

message UpdateRuleRequest {
  ConditionsInput conditions = 1;
  google.protobuf.StringValue params = 2;
}

message ConditionsInput {
  string key = 1;
}

message UpdateRuleResponse {}
`

func TestCompileOptionalNestedInputsTreatsNullAsAbsent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		input               string
		expectConditions    bool
		expectConditionsKey bool
		expectParams        bool
	}{
		{
			name:             "omitted optional fields stay absent",
			input:            `{}`,
			expectConditions: false,
			expectParams:     false,
		},
		{
			name:             "null optional fields stay absent",
			input:            `{"conditions":null,"params":null}`,
			expectConditions: false,
			expectParams:     false,
		},
		{
			name:                "explicit empty object keeps nested message",
			input:               `{"conditions":{}}`,
			expectConditions:    true,
			expectConditionsKey: false,
			expectParams:        false,
		},
		{
			name:                "explicit nested value is preserved",
			input:               `{"conditions":{"key":"status"},"params":"{\"value\":\"1212\"}"}`,
			expectConditions:    true,
			expectConditionsKey: true,
			expectParams:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			compiler, err := NewProtoCompiler(protoSchemaWithOptionalNestedInputs, nil)
			require.NoError(t, err)

			inputMessage, ok := compiler.doc.MessageByName("UpdateRuleRequest")
			require.True(t, ok)

			request, err := compiler.buildProtoMessage(inputMessage, optionalNestedInputsMessage(), gjson.Parse(tc.input))
			require.NoError(t, err)

			conditionsDesc := request.Descriptor().Fields().ByName(protoreflect.Name("conditions"))
			require.NotNil(t, conditionsDesc)
			require.Equal(t, tc.expectConditions, request.Has(conditionsDesc))

			if tc.expectConditions {
				conditions := request.Get(conditionsDesc).Message()
				keyDesc := conditions.Descriptor().Fields().ByName(protoreflect.Name("key"))
				require.NotNil(t, keyDesc)
				require.Equal(t, tc.expectConditionsKey, conditions.Has(keyDesc))
				if tc.expectConditionsKey {
					require.Equal(t, "status", conditions.Get(keyDesc).String())
				}
			}

			paramsDesc := request.Descriptor().Fields().ByName(protoreflect.Name("params"))
			require.NotNil(t, paramsDesc)
			require.Equal(t, tc.expectParams, request.Has(paramsDesc))

			if tc.expectParams {
				params := request.Get(paramsDesc).Message()
				valueDesc := params.Descriptor().Fields().ByName(protoreflect.Name("value"))
				require.NotNil(t, valueDesc)
				require.True(t, params.Has(valueDesc))
				require.Equal(t, `{"value":"1212"}`, params.Get(valueDesc).String())
			}
		})
	}
}

func optionalNestedInputsMessage() *RPCMessage {
	return &RPCMessage{
		Name: "UpdateRuleRequest",
		Fields: RPCFields{
			{
				Name:          "conditions",
				ProtoTypeName: DataTypeMessage,
				JSONPath:      "conditions",
				Optional:      true,
				Message: &RPCMessage{
					Name: "ConditionsInput",
					Fields: RPCFields{
						{
							Name:          "key",
							ProtoTypeName: DataTypeString,
							JSONPath:      "key",
							Optional:      true,
						},
					},
				},
			},
			{
				Name:          "params",
				ProtoTypeName: DataTypeString,
				JSONPath:      "params",
				Optional:      true,
			},
		},
	}
}
