package parser

import (
	"encoding/json"
	"github.com/sebdah/goldie"
	"os"
	"testing"
)

func TestParser(t *testing.T) {
	parser := NewParser()

	starwarsSchema, err := os.Open("../../starwars.schema.graphql")
	if err != nil {
		t.Fatal(err)
	}

	defer starwarsSchema.Close()

	def, err := parser.ParseTypeSystemDefinition(starwarsSchema)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "parser_starwars_typesystemdefinition", jsonBytes)
}
