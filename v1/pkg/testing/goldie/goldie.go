package goldie

import (
	"testing"

	"github.com/sebdah/goldie/v2"
)

// New creates a new instance of Goldie.
func New(t *testing.T) *goldie.Goldie {
	return goldie.New(t, goldie.WithFixtureDir("fixtures"))
}
