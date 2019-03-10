package middleware

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/testhelper"
	"testing"
)

func TestAssetMiddleware(t *testing.T) {

	run := func(schema, queryBefore, queryAfter string) {

		got := middleware.InvokeMiddleware(&AssetUrlMiddleware{}, schema, queryBefore)
		want := testhelper.UglifyRequestString(queryAfter)

		if want != got {
			panic(fmt.Errorf("want:\n%s\ngot:\n%s\n", want, got))
		}
	}

	t.Run("asset handle transform", func(t *testing.T) {
		run(assetSchema,
			`query testQueryWithoutHandle {
  								assets(first: 1) {
    							id
    							fileName
    							url(transformation: {image: {resize: {width: 100, height: 100}}})
  							}
						}`,
			`	query testQueryWithoutHandle {
	  						assets(first: 1) {
    							id
    							fileName
								handle
  							}
						}`)
	})
	t.Run("asset handle transform on empty selectionSet", func(t *testing.T) {
		run(assetSchema,
			`query testQueryWithoutHandle {
  							assets(first: 1) {
  							}
						}`,
			`	query testQueryWithoutHandle {
	  						assets(first: 1) {
    							handle
  							}
						}`)
	})

}

var assetSchema = `
schema {
    query: Query
}

type Query {
    assets(first: Int): [Asset]
}

type Asset implements Node @RequestMiddleware {
    status: Status!
    updatedAt: DateTime!
    createdAt: DateTime!
    id: ID!
    handle: String!
    fileName: String!
    height: Float
    width: Float
    size: Float
    mimeType: String
    url: String!
}`
