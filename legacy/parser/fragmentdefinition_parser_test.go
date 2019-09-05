package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
	"testing"
)

func TestParser_parseFragmentDefinition(t *testing.T) {
	t.Run("simple fragment definition", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType @rename(index: 3){
					name
				}`,
			mustParseFragmentDefinition(
				node(
					hasName("MyFragment"),
					hasTypeName("SomeType"),
					hasFields(
						node(
							hasName("name"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 5,
						LineEnd:   4,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("fragment without optional directives", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType{
					name
				}`,
			mustParseFragmentDefinition(
				node(
					hasName("MyFragment"),
					hasTypeName("SomeType"),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			))
	})
	t.Run("fragment with untyped inline fragment", func(t *testing.T) {
		run(`	fragment inlineFragment2 on Dog {
  						... @include(if: true) {
    						name
  						}
					}`,
			mustParseFragmentDefinition(
				node(
					hasTypeName("Dog"),
					hasInlineFragments(
						node(
							hasDirectives(
								node(
									hasName("include"),
								),
							),
							hasFields(
								node(
									hasName("name"),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid fragment 1", func(t *testing.T) {
		run(`
				fragment MyFragment SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 2", func(t *testing.T) {
		run(`
				fragment MyFragment un SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 3", func(t *testing.T) {
		run(`
				fragment 1337 on SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 4", func(t *testing.T) {
		run(`
				fragment Fields on [SomeType! {
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 4", func(t *testing.T) {
		run(`
				fragment Fields on SomeType @foo(bar: .) {
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
}
