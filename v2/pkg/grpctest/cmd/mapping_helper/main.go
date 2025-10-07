package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"text/template"
)

type JSONMapping struct {
	Version           int                `json:"version"`
	Service           string             `json:"service"`
	OperationMappings []OperationMapping `json:"operationMappings"`
	EntityMappings    []EntityMapping    `json:"entityMappings"`
	TypeFieldMappings []TypeFieldMapping `json:"typeFieldMappings"`
	EnumMappings      []EnumMapping      `json:"enumMappings"`
	ResolveMappings   []ResolveMapping   `json:"resolveMappings"`
}

type OperationMapping struct {
	Type     string `json:"type"`
	Original string `json:"original"`
	Mapped   string `json:"mapped"`
	Request  string `json:"request"`
	Response string `json:"response"`
}

type EntityMapping struct {
	TypeName string `json:"typeName"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	RPC      string `json:"rpc"`
	Request  string `json:"request"`
	Response string `json:"response"`
}

type ResolveMapping struct {
	Type          string        `json:"type"`
	LookupMapping LookupMapping `json:"lookupMapping"`
	RPC           string        `json:"rpc"`
	Request       string        `json:"request"`
	Response      string        `json:"response"`
}

type LookupMapping struct {
	Type         string       `json:"type"`
	FieldMapping FieldMapping `json:"fieldMapping"`
}

type TypeFieldMapping struct {
	Type          string         `json:"type"`
	FieldMappings []FieldMapping `json:"fieldMappings"`
}

type FieldMapping struct {
	Original         string            `json:"original"`
	Mapped           string            `json:"mapped"`
	ArgumentMappings []ArgumentMapping `json:"argumentMappings"`
}

type ArgumentMapping struct {
	Original string `json:"original"`
	Mapped   string `json:"mapped"`
}

type EnumMapping struct {
	Type   string      `json:"type"`
	Values []EnumValue `json:"values"`
}

type EnumValue struct {
	Original string `json:"original"`
	Mapped   string `json:"mapped"`
}

const tpl = `package mapping

import (
	"testing"

	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
)

// DefaultGRPCMapping returns a hardcoded default mapping between GraphQL and Protobuf
func DefaultGRPCMapping() *grpcdatasource.GRPCMapping {
	return &grpcdatasource.GRPCMapping{
		Service: "{{.Service}}",
		QueryRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.RPCConfig]{
		{{- range $index, $operation := .OperationMappings}}
		{{- if eq $operation.Type "OPERATION_TYPE_QUERY"}}
			"{{$operation.Original}}": {
				RPC:      "{{$operation.Mapped}}",
				Request:  "{{$operation.Request}}",
				Response: "{{$operation.Response}}",
			},
		{{- end }}
		{{- end }}
		},
		MutationRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.RPCConfig]{
		{{- range $index, $operation := .OperationMappings}}
		{{- if eq $operation.Type "OPERATION_TYPE_MUTATION"}}
			"{{$operation.Original}}": {
				RPC:      "{{$operation.Mapped}}",
				Request:  "{{$operation.Request}}",
				Response: "{{$operation.Response}}",
			},
		{{- end }}
		{{- end }}
		},
		SubscriptionRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.RPCConfig]{
		{{- range $index, $operation := .OperationMappings}}
		{{- if eq $operation.Type "OPERATION_TYPE_SUBSCRIPTION"}}
			"{{$operation.Original}}": {
				RPC:      "{{$operation.Mapped}}",
				Request:  "{{$operation.Request}}",
				Response: "{{$operation.Response}}",
			},
		{{- end }}
		{{- end }}
		},
		ResolveRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.ResolveRPCMapping]{
		{{- range $index, $resolve := .ResolveMappings}}
		{{- if eq $resolve.Type "LOOKUP_TYPE_RESOLVE"}}
			"{{$resolve.LookupMapping.Type}}": {
				"{{$resolve.LookupMapping.FieldMapping.Original}}": {
					FieldMappingData: grpcdatasource.FieldMapData{
						TargetName: "{{$resolve.LookupMapping.FieldMapping.Mapped}}",
						ArgumentMappings: grpcdatasource.FieldArgumentMap{
							{{- range $index, $argument := $resolve.LookupMapping.FieldMapping.ArgumentMappings}}
								"{{$argument.Original}}": "{{$argument.Mapped}}",
							{{- end }}
						},
					},
					RPC:      "{{$resolve.RPC}}",
					Request:  "{{$resolve.Request}}",
					Response: "{{$resolve.Response}}",
				},
			},
		{{- end }}
		{{- end }}
		},
		EntityRPCs: map[string][]grpcdatasource.EntityRPCConfig{
		{{- range $index, $entity := .EntityMappings}}
			"{{$entity.TypeName}}": {
				{
					Key: "{{$entity.Key}}",
					RPCConfig: grpcdatasource.RPCConfig{
						RPC:      "{{$entity.RPC}}",
						Request:  "{{$entity.Request}}",
						Response: "{{$entity.Response}}",
					},
				},
			},
		{{- end }}
		},
		EnumValues: map[string][]grpcdatasource.EnumValueMapping{
		{{- range $index, $enum := .EnumMappings}}
			"{{$enum.Type}}": {
				{{- range $index, $value := .Values}}
					{Value: "{{$value.Original}}", TargetValue: "{{$value.Mapped}}"},
				{{- end }}
			},
		{{- end }}
		},
		Fields: map[string]grpcdatasource.FieldMap{
		{{- range $index, $typeField := .TypeFieldMappings}}
		"{{$typeField.Type}}": {
			{{- range $index, $field := $typeField.FieldMappings}}
			"{{$field.Original}}": {
				TargetName: "{{$field.Mapped}}", 
				{{- if (gt (len $field.ArgumentMappings) 0)}}
				ArgumentMappings: grpcdatasource.FieldArgumentMap{
				{{- range $index, $argument := $field.ArgumentMappings}}
					"{{$argument.Original}}": "{{$argument.Mapped}}",
				{{- end }}
				},
				{{- end }}
			},
			{{- end}}
			},
		{{- end}}
		},
	}
}


// MustDefaultGRPCMapping returns the default GRPC mapping
func MustDefaultGRPCMapping(t *testing.T) *grpcdatasource.GRPCMapping {
	mapping := DefaultGRPCMapping()
	return mapping
}


`

func main() {
	args := os.Args[1:]
	if len(args) < 2 {
		log.Fatal("No input file or output file provided")
	}

	inputFile := args[0]
	outputFile := args[1]

	jsonBytes, err := os.ReadFile(inputFile)
	if err != nil {
		log.Fatal(err)
	}
	var mapping JSONMapping
	err = json.Unmarshal(jsonBytes, &mapping)
	if err != nil {
		log.Fatal(err)
	}

	t := template.Must(template.New("mapping").Parse(tpl))

	buf := &bytes.Buffer{}
	if err := t.Execute(buf, mapping); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(outputFile, buf.Bytes(), 0644); err != nil {
		log.Fatal(err)
	}
}
