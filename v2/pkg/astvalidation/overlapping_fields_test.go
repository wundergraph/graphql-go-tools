package astvalidation

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"
	"text/template"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// inspired by: https://tech.xing.com/graphql-overlapping-fields-can-be-merged-fast-ea6e92e0a01
func TestOverlappingFieldsCanBeMerged(t *testing.T) {

	t.Run("valid", func(t *testing.T) {
		definitionBytes := RenderTemplate(5, OverlappingFieldsDefinition)
		operationBytes := RenderTemplate(5, OverlappingFieldsOperationValid)

		definition := unsafeparser.ParseGraphqlDocumentBytes(definitionBytes)
		operation := unsafeparser.ParseGraphqlDocumentBytes(operationBytes)
		report := operationreport.Report{}
		normalizer := astnormalization.NewNormalizer(false, false)
		validator := DefaultOperationValidator()

		normalizer.NormalizeOperation(&operation, &definition, &report)
		if report.HasErrors() {
			panic(report.Error())
		}

		if validator.Validate(&operation, &definition, &report) != Valid {
			panic(report.Error())
		}
	})
	t.Run("invalid", func(t *testing.T) {
		definitionBytes := RenderTemplate(5, OverlappingFieldsDefinition)
		operationBytes := RenderTemplate(5, OverlappingFieldsOperationInvalid)

		fmt.Println(string(operationBytes))

		definition := unsafeparser.ParseGraphqlDocumentBytes(definitionBytes)
		operation := unsafeparser.ParseGraphqlDocumentBytes(operationBytes)
		report := operationreport.Report{}
		normalizer := astnormalization.NewNormalizer(false, false)
		validator := DefaultOperationValidator()

		normalizer.NormalizeOperation(&operation, &definition, &report)
		if report.HasErrors() {
			panic(report.Error())
		}

		if validator.Validate(&operation, &definition, &report) != Invalid {
			panic(report.Error())
		}
	})
}

func BenchmarkOverlappingFieldsCanBeMerged(b *testing.B) {
	multipliers := []int{1, 5, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 200}
	for _, multiplier := range multipliers {
		multiplier := multiplier
		b.Run("valid"+strconv.Itoa(multiplier), func(b *testing.B) {
			definitionBytes := RenderTemplate(multiplier, OverlappingFieldsDefinition)
			operationBytes := RenderTemplate(multiplier, OverlappingFieldsOperationValid)

			definition := unsafeparser.ParseGraphqlDocumentBytes(definitionBytes)
			operation := unsafeparser.ParseGraphqlDocumentBytes(operationBytes)
			report := operationreport.Report{}
			normalizer := astnormalization.NewNormalizer(false, false)
			validator := DefaultOperationValidator()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				report.Reset()
				definition := definition
				normalizer.NormalizeOperation(&operation, &definition, &report)
				if report.HasErrors() {
					panic(report.Error())
				}

				if validator.Validate(&operation, &definition, &report) != Valid {
					panic(report.Error())
				}
			}
		})
		b.Run("invalid"+strconv.Itoa(multiplier), func(b *testing.B) {
			definitionBytes := RenderTemplate(multiplier, OverlappingFieldsDefinition)
			operationBytes := RenderTemplate(multiplier, OverlappingFieldsOperationInvalid)

			definition := unsafeparser.ParseGraphqlDocumentBytes(definitionBytes)
			operation := unsafeparser.ParseGraphqlDocumentBytes(operationBytes)
			report := operationreport.Report{}
			normalizer := astnormalization.NewNormalizer(false, false)
			validator := DefaultOperationValidator()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				report.Reset()
				definition := definition
				normalizer.NormalizeOperation(&operation, &definition, &report)
				if report.HasErrors() {
					panic(report.Error())
				}

				if validator.Validate(&operation, &definition, &report) != Invalid {
					panic(report.Error())
				}
			}
		})
	}
}

func RenderTemplate(multiplier int, tmpl string) []byte {
	t := template.Must(template.New("tmpl").Parse(tmpl))
	data := make([]int, 0, multiplier)
	for i := 0; i < multiplier; i++ {
		data = append(data, i+1)
	}
	buff := bytes.Buffer{}
	err := t.Execute(&buff, data)
	if err != nil {
		panic(err)
	}
	return buff.Bytes()
}

const OverlappingFieldsOperationValid = `
{
	someBox {
		... on StringBox {
			scalar
		}{{ range $i := . }}
		... on StringBox{{ $i }} {
			scalar
		}{{ end }}
	}
}`

const OverlappingFieldsOperationInvalid = `
{
	someBox {
		... on StringBox {
			scalar
		}{{ range $i := . }}
		... on StringBox{{ $i }} {
			scalar
		}{{ end }}
		... on NonNullStringBox1 {
			scalar
		}
	}
}`

const OverlappingFieldsDefinition = `
scalar String
scalar ID
scalar Int
interface SomeBox {
	deepBox: SomeBox
	unrelatedField: String
}
type StringBox implements SomeBox {
	scalar: String
	deepBox: StringBox
	unrelatedField: String
	listStringBox: [StringBox]
	stringBox: StringBox
	intBox: IntBox
}
{{ range $i := . }}{{ if ne $i 1 }}
{{ end}}type StringBox{{ $i }} implements SomeBox {
	scalar: String
	deepBox: StringBox
	unrelatedField: String
	listStringBox: [StringBox]
	stringBox: StringBox
	intBox: IntBox
}{{ end }}
type IntBox implements SomeBox {
	scalar: Int
	deepBox: IntBox
	unrelatedField: String
	listStringBox: [StringBox]
	stringBox: StringBox
	intBox: IntBox
}
interface NonNullStringBox1 {
	scalar: String!
}
type NonNullStringBox1Impl implements SomeBox & NonNullStringBox1 {
	scalar: String!
	unrelatedField: String
	deepBox: SomeBox
}
interface NonNullStringBox2 {
	scalar: String!
}
type NonNullStringBox2Impl implements SomeBox & NonNullStringBox2 {
	scalar: String!
	unrelatedField: String
	deepBox: SomeBox
}
type Connection {
	edges: [Edge]
}
type Edge {
	node: Node
}
type Node {
	id: ID
	name: String
}
type Query {
	someBox: SomeBox
	connection: Connection
}
schema {
	query: Query
}
`
