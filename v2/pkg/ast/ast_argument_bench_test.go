package ast_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
)

// Cover one comparison and sixteen selections (120 comparisons), including
// nested input objects and the fallback for arguments in different orders.
func BenchmarkArgumentSetsAreEqualUnordered(b *testing.B) {
	for _, fieldCount := range []int{2, 16} {
		for _, nested := range []bool{false, true} {
			for _, size := range []int{0, 1, 4, 32, 128} {
				if nested && size == 0 {
					continue
				}
				for _, ordered := range []bool{true, false} {
					if !ordered && size < 2 {
						continue
					}
					b.Run(fmt.Sprintf("fields=%d/object=%t/size=%d/ordered=%t", fieldCount, nested, size, ordered), func(b *testing.B) {
						var query strings.Builder
						query.WriteByte('{')
						for field := 0; field < fieldCount; field++ {
							query.WriteString("field")
							if size > 0 {
								query.WriteByte('(')
								if nested {
									query.WriteString("input: {")
								}
								for arg := 0; arg < size; arg++ {
									index := arg
									if !ordered {
										index = (arg + field) % size
									}
									fmt.Fprintf(&query, "a%03d: %d ", index, index)
								}
								if nested {
									query.WriteByte('}')
								}
								query.WriteByte(')')
							}
							query.WriteByte(' ')
						}
						query.WriteByte('}')
						document, report := astparser.ParseGraphqlDocumentString(query.String())
						if report.HasErrors() {
							b.Fatal(report.Error())
						}
						b.ReportAllocs()
						for b.Loop() {
							for left := 0; left < fieldCount; left++ {
								for right := left + 1; right < fieldCount; right++ {
									if !document.ArgumentSetsAreEqualUnordered(document.FieldArguments(left), document.FieldArguments(right)) {
										b.Fatal("unequal")
									}
								}
							}
						}
					})
				}
			}
		}
	}
}
