package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseType(t *testing.T) {
	t.Run("simple named", func(t *testing.T) {
		run("String", mustParseType(
			hasTypeKind(document.TypeKindNAMED),
			hasTypeName("String"),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   7,
			}),
		))
	})
	t.Run("named non null", func(t *testing.T) {
		run("String!", mustParseType(
			hasTypeKind(document.TypeKindNON_NULL),
			ofType(
				hasTypeKind(document.TypeKindNAMED),
				hasTypeName("String"),
			),
		))
	})
	t.Run("non null named list", func(t *testing.T) {
		run("[String!]", mustParseType(
			hasTypeKind(document.TypeKindLIST),
			ofType(
				hasTypeKind(document.TypeKindNON_NULL),
				ofType(
					hasTypeKind(document.TypeKindNAMED),
					hasTypeName("String"),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   10,
			}),
		))
	})
	t.Run("non null named non null list", func(t *testing.T) {
		run("[String!]!", mustParseType(
			hasTypeKind(document.TypeKindNON_NULL),
			ofType(
				hasTypeKind(document.TypeKindLIST),
				ofType(
					hasTypeKind(document.TypeKindNON_NULL),
					ofType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
				),
			),
		))
	})
	t.Run("nested list", func(t *testing.T) {
		run("[[[String]!]]", mustParseType(
			hasTypeKind(document.TypeKindLIST),
			ofType(
				hasTypeKind(document.TypeKindLIST),
				ofType(
					hasTypeKind(document.TypeKindNON_NULL),
					ofType(
						hasTypeKind(document.TypeKindLIST),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
							hasPosition(position.Position{
								LineStart: 1,
								CharStart: 4,
								LineEnd:   1,
								CharEnd:   10,
							}),
						),
					),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   14,
			}),
		))
	})
	t.Run("invalid", func(t *testing.T) {
		run("[\"String\"]",
			mustPanic(
				mustParseType(
					hasTypeKind(document.TypeKindLIST),
					ofType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				),
			),
		)
	})
}
