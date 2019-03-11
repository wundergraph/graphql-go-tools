package parser

import "testing"

func TestParser_parseUnionTypeDefinition(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run("union SearchResult = Photo | Person",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person"),
				),
			))
	})
	t.Run("multiple members", func(t *testing.T) {
		run("union SearchResult = Photo | Person | Car | Planet",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person", "Car", "Planet"),
				),
			),
		)
	})
	t.Run("with linebreaks", func(t *testing.T) {
		run(`union SearchResult = Photo 
										| Person 
										| Car 
										| Planet`,
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person", "Car", "Planet"),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`union SearchResult @fromTop(to: "bottom") @fromBottom(to: "top") = Photo | Person`,
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasUnionMemberTypes("Photo", "Person"),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("union 1337 = Photo | Person",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("1337"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("union SearchResult @foo(bar: .) = Photo | Person",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("union SearchResult = Photo | Person | 1337",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person", "1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("union SearchResult = Photo | Person | \"Video\"",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 5", func(t *testing.T) {
		run("notunion SearchResult = Photo | Person",
			mustPanic(mustParseUnionTypeDefinition()),
		)
	})
}
