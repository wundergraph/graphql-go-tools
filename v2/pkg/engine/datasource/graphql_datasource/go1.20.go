//go:build !go1.21

package graphql_datasource

import (
	"encoding/json"
	"regexp"

	"github.com/buger/jsonparser"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// XXX: This is a workaround for Go 1.20 and below not supporting encoding/decoding
// of regexp.Regexp.

var _ interface {
	json.Marshaler
	json.Unmarshaler
} = (*SubscriptionConfiguration)(nil)

func (c SubscriptionConfiguration) MarshalJSON() ([]byte, error) {
	// Use an alias to avoid recursively calling into ourselves
	type subscriptionConfiguration SubscriptionConfiguration
	cpy := subscriptionConfiguration(c)
	data, err := json.Marshal(cpy)
	if err != nil {
		return nil, err
	}
	regexps := make([]string, 0, len(c.ForwardedClientHeaderRegularExpressions))
	for _, re := range c.ForwardedClientHeaderRegularExpressions {
		regexps = append(regexps, re.String())
	}
	regexpsData, err := json.Marshal(regexps)
	if err != nil {
		return nil, err
	}
	return jsonparser.Set(data, regexpsData, "ForwardedClientHeaderRegularExpressions")
}

func (c *SubscriptionConfiguration) UnmarshalJSON(data []byte) error {
	regexps := gjson.GetBytes(data, "ForwardedClientHeaderRegularExpressions").Array()
	data, err := sjson.DeleteBytes(data, "ForwardedClientHeaderRegularExpressions")
	if err != nil {
		return err
	}
	type subscriptionConfiguration SubscriptionConfiguration
	var configuration subscriptionConfiguration
	if err := json.Unmarshal(data, &configuration); err != nil {
		return err
	}
	for _, value := range regexps {
		r, err := regexp.Compile(value.Str)
		if err != nil {
			return err
		}
		configuration.ForwardedClientHeaderRegularExpressions = append(configuration.ForwardedClientHeaderRegularExpressions, r)
	}
	*c = SubscriptionConfiguration(configuration)
	return nil
}

var _ json.Unmarshaler = (*GraphQLSubscriptionOptions)(nil)

func (opts *GraphQLSubscriptionOptions) UnmarshalJSON(data []byte) error {
	regexps := gjson.GetBytes(data, "forwarded_client_header_regular_expressions").Array()
	data, err := sjson.DeleteBytes(data, "forwarded_client_header_regular_expressions")
	if err != nil {
		return err
	}
	type graphQLSubscriptionOptions GraphQLSubscriptionOptions
	var options graphQLSubscriptionOptions
	if err := json.Unmarshal(data, &options); err != nil {
		return err
	}
	for _, value := range regexps {
		r, err := regexp.Compile(value.Str)
		if err != nil {
			return err
		}
		options.ForwardedClientHeaderRegularExpressions = append(options.ForwardedClientHeaderRegularExpressions, r)
	}
	*opts = GraphQLSubscriptionOptions(options)
	return nil
}
