package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestRealDepthCalculator_CalculateDepthForFragmentSpread(t *testing.T) {
	run := func(t *testing.T, operation, definition, spreadName string, wantDepth int) {
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		def := unsafeparser.ParseGraphqlDocumentString(definition)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			panic(err)
		}

		report := operationreport.Report{}
		calc := FragmentSpreadDepth{}
		var depths Depths
		calc.Get(&op, &def, &report, &depths)
		if report.HasErrors() {
			panic(report.Error())
		}

		gotDepth := -1
		for _, depth := range depths {
			if string(depth.SpreadName) == spreadName {
				gotDepth = depth.Depth
				break
			}
		}

		assert.Equal(t, wantDepth, gotDepth)
	}

	t.Run("simple", func(t *testing.T) {
		run(t, `
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`, testDefinition, "frag1", 3)
	})
	t.Run("nested", func(t *testing.T) {
		run(t, `
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`, testDefinition, "frag2", 6)
	})
}

func BenchmarkFragmentSpreadDepthCalc_Get(b *testing.B) {
	nested := `
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`

	op := unsafeparser.ParseGraphqlDocumentString(nested)
	def := unsafeparser.ParseGraphqlDocumentString(testDefinition)

	calc := &FragmentSpreadDepth{}
	depths := make(Depths, 0, 8)
	report := operationreport.Report{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		depths = depths[:0]
		calc.Get(&op, &def, &report, &depths)
	}
}
