package parser

import "testing"

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
