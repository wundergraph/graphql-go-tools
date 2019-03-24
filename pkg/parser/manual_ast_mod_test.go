package parser

import "testing"

func TestManualAstMod_PutLiteralBytes(t *testing.T) {
	parser := NewParser()
	mod := NewManualAstMod(parser)
	input := make([]byte, 1000000+1)
	_, _, err := mod.PutLiteralBytes(input)
	if err == nil {
		panic("want err")
	}
}

func TestManualAstMod_MergeArgIntoFieldArguments(t *testing.T) {
	t.Run("merge argument into field without arguments", func(t *testing.T) {
		run(`query myDocuments {
						documents {
							sensitiveInformation
						}
					}`, mustParseOperationDefinition(),
			mustMergeArgumentOnField("sensitiveInformation", "user", `"jsmith@example.org"`),
			mustContainOperationDefinition(
				node(
					hasFields(
						node(
							hasName("documents"),
							hasFields(
								node(
									hasName("sensitiveInformation"),
									hasArguments(
										node(
											hasName("user"),
											hasValue(hasRawValueContent("jsmith@example.org")),
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
	t.Run("merge argument into field with existing argument", func(t *testing.T) {
		run(`query myDocuments {
						documents {
							sensitiveInformation(foo: "bar")
						}
					}`, mustParseOperationDefinition(),
			mustMergeArgumentOnField("sensitiveInformation", "user", `"jsmith@example.org"`),
			mustContainOperationDefinition(
				node(
					hasFields(
						node(
							hasName("documents"),
							hasFields(
								node(
									hasName("sensitiveInformation"),
									hasArguments(
										node(
											hasName("foo"),
											hasValue(hasRawValueContent("bar")),
										),
										node(
											hasName("user"),
											hasValue(hasRawValueContent("jsmith@example.org")),
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
	t.Run("merge argument into field with existing argument of same name", func(t *testing.T) {
		run(`query myDocuments {
						documents {
							sensitiveInformation(user: "bar")
						}
					}`, mustParseOperationDefinition(),
			mustMergeArgumentOnField("sensitiveInformation", "user", `"jsmith@example.org"`),
			mustContainOperationDefinition(
				node(
					hasFields(
						node(
							hasName("documents"),
							hasFields(
								node(
									hasName("sensitiveInformation"),
									hasArguments(
										node(
											hasName("user"),
											hasValue(hasRawValueContent("jsmith@example.org")),
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
	t.Run("merge argument into field with existing argument of same name (reverse test)", func(t *testing.T) {
		run(`query myDocuments {
						documents {
							sensitiveInformation(user: "bar")
						}
					}`, mustParseOperationDefinition(),
			mustMergeArgumentOnField("sensitiveInformation", "user", `"jsmith@example.org"`),
			mustPanic(mustContainOperationDefinition(
				node(
					hasFields(
						node(
							hasName("documents"),
							hasFields(
								node(
									hasName("sensitiveInformation"),
									hasArguments(
										node(
											hasName("user"),
											hasValue(hasRawValueContent("bar")),
										),
										node(
											hasName("user"),
											hasValue(hasRawValueContent("jsmith@example.org")),
										),
									),
								),
							),
						),
					),
				),
			)),
		)
	})
}

func TestManualAstMod_AppendFieldToSelectionSet(t *testing.T) {
	t.Run("with existing field", func(t *testing.T) {
		run(`{foo}`,
			mustParseSelectionSet(
				node(
					hasFields(
						node(hasName("foo")),
					),
				),
			),
			mustAppendFieldToSelectionSet(0, "bar"),
			mustContainSelectionSet(0, node(
				hasFields(
					node(hasName("foo")),
					node(hasName("bar")),
				),
			)),
		)
	})
	t.Run("with existing field of same name", func(t *testing.T) {
		run(`{foo}`,
			mustParseSelectionSet(
				node(
					hasFields(
						node(hasName("foo")),
					),
				),
			),
			mustAppendFieldToSelectionSet(0, "foo"),
			mustContainSelectionSet(0, node(
				hasFields(
					node(hasName("foo")),
					node(hasName("foo")),
				),
			)),
		)
	})
	t.Run("negative test", func(t *testing.T) {
		run(`{foo}`,
			mustParseSelectionSet(
				node(
					hasFields(
						node(hasName("foo")),
					),
				),
			),
			mustAppendFieldToSelectionSet(0, "foo"),
			mustPanic(mustContainSelectionSet(0, node(
				hasFields(
					node(hasName("foo")),
					node(hasName("foo")),
					node(hasName("foo")),
				),
			))),
		)
	})
}

func TestManualAstMod_DeleteFieldFromSelectionSet(t *testing.T) {
	t.Run("delete second field of three fields", func(t *testing.T) {
		run(`{foo bar baz}`,
			mustParseSelectionSet(
				node(
					hasFields(
						node(hasName("foo")),
						node(hasName("bar")),
						node(hasName("baz")),
					),
				),
			),
			mustDeleteFieldFromSelectionSet(0, 1),
			mustContainSelectionSet(0, node(
				hasFields(
					node(hasName("foo")),
					node(hasName("baz")),
				),
			)),
		)
	})
	t.Run("delete first field of three fields", func(t *testing.T) {
		run(`{foo bar baz}`,
			mustParseSelectionSet(
				node(
					hasFields(
						node(hasName("foo")),
						node(hasName("bar")),
						node(hasName("baz")),
					),
				),
			),
			mustDeleteFieldFromSelectionSet(0, 0),
			mustContainSelectionSet(0, node(
				hasFields(
					node(hasName("bar")),
					node(hasName("baz")),
				),
			)),
		)
	})
	t.Run("delete third field of three fields", func(t *testing.T) {
		run(`{foo bar baz}`,
			mustParseSelectionSet(
				node(
					hasFields(
						node(hasName("foo")),
						node(hasName("bar")),
						node(hasName("baz")),
					),
				),
			),
			mustDeleteFieldFromSelectionSet(0, 2),
			mustContainSelectionSet(0, node(
				hasFields(
					node(hasName("foo")),
					node(hasName("bar")),
				),
			)),
		)
	})
	t.Run("negative test", func(t *testing.T) {
		run(`{foo bar baz}`,
			mustParseSelectionSet(
				node(
					hasFields(
						node(hasName("foo")),
						node(hasName("bar")),
						node(hasName("baz")),
					),
				),
			),
			mustDeleteFieldFromSelectionSet(0, 2),
			mustPanic(mustContainSelectionSet(0, node(
				hasFields(
					node(hasName("foo")),
					node(hasName("bar")),
					node(hasName("baz")),
				),
			))),
		)
	})
}
