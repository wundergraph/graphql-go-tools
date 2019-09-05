package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseInlineFragment(t *testing.T) {
	t.Run("with nested selectionsets", func(t *testing.T) {
		run(`on Goland {
					... on GoWater {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustParseInlineFragments(
				node(
					hasTypeName("Goland"),
					hasPosition(position.Position{
						LineStart: 0, // default, see mustParseFragmentSpread
						CharStart: 0, // default, see mustParseFragmentSpread
						LineEnd:   7,
						CharEnd:   6,
					}),
					hasInlineFragments(
						node(
							hasTypeName("GoWater"),
							hasPosition(position.Position{
								LineStart: 2,
								CharStart: 6,
								LineEnd:   6,
								CharEnd:   7,
							}),
							hasInlineFragments(
								node(
									hasTypeName("GoAir"),
									hasPosition(position.Position{
										LineStart: 3,
										CharStart: 7,
										LineEnd:   5,
										CharEnd:   8,
									}),
									hasFields(
										node(
											hasName("go"),
										),
									),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("inline fragment without type condition", func(t *testing.T) {
		run(`	@include(if: true) {
    					name
					}`,
			mustParseInlineFragments(
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
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`on Goland {
					... on 1337 {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustPanic(
				mustParseInlineFragments(
					node(
						hasTypeName("\"Goland\""),
						hasInlineFragments(
							node(
								hasTypeName("1337"),
								hasInlineFragments(
									node(
										hasTypeName("GoAir"),
										hasFields(
											node(
												hasName("go"),
											),
										),
									),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`on Goland {
					... on GoWater @foo(bar: .) {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustPanic(
				mustParseInlineFragments(
					node(
						hasTypeName("Goland"),
						hasInlineFragments(
							node(
								hasTypeName("GoWater"),
								hasInlineFragments(
									node(
										hasTypeName("GoAir"),
										hasFields(
											node(
												hasName("go"),
											),
										),
									),
								),
							),
						),
					),
				),
			))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`	on Goland {
						... on [Water] {
							waterField
						}
					}`, mustPanic(mustParseInlineFragments(node())))
	})
}
