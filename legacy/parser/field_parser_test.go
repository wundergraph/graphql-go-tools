package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseField(t *testing.T) {
	t.Run("parse field with name, arguments and directive", func(t *testing.T) {
		run("preferredName: originalName(isSet: true) @rename(index: 3)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasArguments(
						node(
							hasName("isSet"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   59,
					}),
				),
			),
		)
	})
	t.Run("without optional alias", func(t *testing.T) {
		run("originalName(isSet: true) @rename(index: 3)",
			mustParseFields(
				node(
					hasName("originalName"),
					hasArguments(
						node(hasName("isSet")),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
				),
			),
		)
	})
	t.Run("without optional arguments", func(t *testing.T) {
		run("preferredName: originalName @rename(index: 3)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
				),
			),
		)
	})
	t.Run("without optional directives", func(t *testing.T) {
		run("preferredName: originalName(isSet: true)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasArguments(
						node(
							hasName("isSet"),
						),
					),
				),
			),
		)
	})
	t.Run("with nested selection sets", func(t *testing.T) {
		run(`
				level1 {
					level2 {
						level3
					}
				}
				`,
			mustParseFields(
				node(
					hasName("level1"),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 5,
						LineEnd:   6,
						CharEnd:   6,
					}),
					hasFields(
						node(
							hasName("level2"),
							hasPosition(position.Position{
								LineStart: 3,
								CharStart: 6,
								LineEnd:   5,
								CharEnd:   7,
							}),
							hasFields(
								node(
									hasName("level3"),
									hasPosition(position.Position{
										LineStart: 4,
										CharStart: 7,
										LineEnd:   4,
										CharEnd:   13,
									}),
								),
							),
						),
					),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`
				level1 {
					alis: .
				}
				`,
			mustPanic(
				mustParseFields(
					node(
						hasName("level1"),
						hasFields(
							node(
								hasAlias("alias"),
								hasName("."),
							),
						),
					),
				)))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`
				level1 {
					alis: ok @foo(bar: .)
				}
				`,
			mustPanic(
				mustParseFields(
					node(
						hasName("level1"),
						hasFields(
							node(
								hasAlias("alias"),
								hasName("ok"),
							),
						),
					),
				)))
	})
}
