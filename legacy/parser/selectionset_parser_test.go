package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
	"testing"
)

func TestParser_parseSelectionSet(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(`{ foo }`, mustParseSelectionSet(
			node(
				hasFields(
					node(
						hasName("foo"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   8,
				}),
			),
		))
	})
	t.Run("inline and fragment spreads", func(t *testing.T) {
		run(`{
					... on Goland
					...Air
					... on Water
				}`,
			mustParseSelectionSet(
				node(
					hasInlineFragments(
						node(
							hasTypeName("Goland"),
						),
						node(
							hasTypeName("Water"),
						),
					),
					hasFragmentSpreads(
						node(
							hasName("Air"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   5,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("mixed", func(t *testing.T) {
		run(`{
					... on Goland
					preferredName: originalName(isSet: true)
					... on Water
				}`, mustParseSelectionSet(
			node(
				hasFields(
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
				hasInlineFragments(
					node(
						hasTypeName("Goland"),
					),
					node(
						hasTypeName("Water"),
					),
				),
			),
		))
	})
	t.Run("field with directives", func(t *testing.T) {
		run(`{
					preferredName: originalName(isSet: true) @rename(index: 3)
				}`, mustParseSelectionSet(
			node(
				hasFields(
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
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   3,
					CharEnd:   6,
				}),
			),
		))
	})
	t.Run("fragment with directive", func(t *testing.T) {
		run(`{
					...firstFragment @rename(index: 3)
				}`, mustParseSelectionSet(
			node(
				hasFragmentSpreads(
					node(
						hasName("firstFragment"),
						hasDirectives(
							node(
								hasName("rename"),
							),
						),
					),
				),
			),
		))
	})
	t.Run("invalid", func(t *testing.T) {
		run(`{
					...firstFragment @rename(index: .)
				}`,
			mustPanic(
				mustParseSelectionSet(
					node(
						hasFragmentSpreads(
							node(
								hasName("firstFragment"),
								hasDirectives(
									node(
										hasName("rename"),
									),
								),
							),
						),
					),
				),
			),
		)
	})
}
