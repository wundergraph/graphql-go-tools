//go:build !go1.21

package graphql_datasource

import (
	"regexp"
	"testing"

	"encoding/json"

	"github.com/stretchr/testify/require"
)

// Make sure we can serialize/deserialize these types with Go 1.20 and below.

func TestSerialization(t *testing.T) {
	sc := SubscriptionConfiguration{
		ForwardedClientHeaderRegularExpressions: []*regexp.Regexp{
			regexp.MustCompile("^foo"),
		},
	}
	data, err := json.Marshal(sc)
	require.NoError(t, err)
	var sc2 SubscriptionConfiguration
	err = json.Unmarshal(data, &sc2)
	require.NoError(t, err)
	require.Equal(t, sc, sc2)
}
