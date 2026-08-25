package introspection

import (
	"os"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
)

func BenchmarkBuildJSON(b *testing.B) {
	starwarsSchema, err := os.ReadFile("./testdata/starwars.schema.graphql")
	if err != nil {
		b.Fatal(err)
	}

	def, report := astparser.ParseGraphqlDocumentBytes(starwarsSchema)
	if report.HasErrors() {
		b.Fatal(report)
	}
	if err := asttransform.MergeDefinitionWithBaseSchema(&def); err != nil {
		b.Fatal(err)
	}

	gen := NewGenerator()
	var data Data
	gen.Generate(&def, &report, &data)
	if report.HasErrors() {
		b.Fatal(report)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := data.Schema.BuildJSON(); err != nil {
			b.Fatal(err)
		}
	}
}
