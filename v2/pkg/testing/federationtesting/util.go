package federationtesting

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	federationTestingDirectoryRelativePath = "pkg/testing/federationtesting"

	QueryReviewsOfMe = `query ReviewsOfMe {
  me {
    reviews {
      body
      product {
        upc
        name
        price
      }
    }
  }
}`
	SubscriptionUpdatedPrice = `subscription UpdatedPrice {
  updatedPrice {
    name
    price
  }
}`
)

type Upstream string

const (
	UpstreamAccounts Upstream = "accounts"
	UpstreamProducts Upstream = "products"
	UpstreamReviews  Upstream = "reviews"
)

func LoadTestingSubgraphSDL(upstream Upstream) ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	absolutePath := filepath.Join(strings.Split(wd, "pkg")[0], federationTestingDirectoryRelativePath, string(upstream), "graph", "schema.graphqls")
	return os.ReadFile(absolutePath)
}
