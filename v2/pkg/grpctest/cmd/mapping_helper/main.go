// mapping_helper is a tool that generates a mapping.go file from a mapping.json file.
// The mapping.go file is used for testing the grpc_datasource package.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"os"
)

type TemplateData struct {
	JSONMapping

	ResolveRPCs map[string][]ResolveRPC
}

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
	TypeName              string                 `json:"typeName"`
	Kind                  string                 `json:"kind"`
	Key                   string                 `json:"key"`
	RPC                   string                 `json:"rpc"`
	Request               string                 `json:"request"`
	Response              string                 `json:"response"`
	RequiredFieldMappings []RequiredFieldMapping `json:"requiredFieldMappings,omitempty"`
}

type RequiredFieldMapping struct {
	FieldMapping FieldMapping `json:"fieldMapping"`
	RPC          string       `json:"rpc"`
	Request      string       `json:"request"`
	Response     string       `json:"response"`
}

type ResolveRPC struct {
	LookupMapping LookupMapping
	RPC           string
	Request       string
	Response      string
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

var (
	//go:embed templates/grpctest_mapping.tmpl
	tpl string
	//go:embed templates/grcp_datasource_mapping_helper.tmpl
	tplWithoutPackage string

	withoutPackage bool
)

// define flags for the command
func init() {
	flag.BoolVar(&withoutPackage, "without-package", false, "generate mapping without package")
}

func main() {
	flag.Parse()

	args := flag.Args()
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

	data := TemplateData{
		JSONMapping: mapping,
		ResolveRPCs: make(map[string][]ResolveRPC),
	}

	for _, mapping := range mapping.ResolveMappings {
		if mapping.Type != "LOOKUP_TYPE_RESOLVE" {
			continue
		}
		t := mapping.LookupMapping.Type
		item, ok := data.ResolveRPCs[t]
		if !ok {
			item = []ResolveRPC{}
		}

		item = append(item, ResolveRPC{
			LookupMapping: mapping.LookupMapping,
			RPC:           mapping.RPC,
			Request:       mapping.Request,
			Response:      mapping.Response,
		})

		data.ResolveRPCs[t] = item
	}

	var usedTemplate = tpl
	if withoutPackage {
		usedTemplate = tplWithoutPackage
	}

	t := template.Must(template.New("mapping").Parse(usedTemplate))

	buf := &bytes.Buffer{}
	if err := t.Execute(buf, data); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(outputFile, buf.Bytes(), 0644); err != nil {
		log.Fatal(err)
	}
}
