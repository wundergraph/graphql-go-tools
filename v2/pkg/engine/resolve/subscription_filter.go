package resolve

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/buger/jsonparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

type SubscriptionFilter struct {
	And []SubscriptionFilter
	Or  []SubscriptionFilter
	Not *SubscriptionFilter
	In  *SubscriptionFieldFilter
}

type SubscriptionFieldFilter struct {
	FieldPath []string
	Values    []InputTemplate
}

func (f *SubscriptionFilter) SkipEvent(ctx *Context, data []byte, buf *bytes.Buffer) (bool, error) {
	if f == nil {
		return false, nil
	}

	if f.And != nil {
		for _, filter := range f.And {
			skip, err := filter.SkipEvent(ctx, data, buf)
			if err != nil {
				return false, err
			}
			/* Skip will be true if any AND predicate is false, so immediately return true
			 * because all AND predicates must be true for the event to be included */
			if skip {
				return true, nil
			}
		}
		return false, nil
	}

	if f.Or != nil {
		for _, filter := range f.Or {
			skip, err := filter.SkipEvent(ctx, data, buf)
			if err != nil {
				return false, err
			}
			/* Skip will be false if any OR predicate is true, so immediately return false
			 * because only a single OR predicate must be true for the event to be included */
			if !skip {
				return false, nil
			}
		}
		return true, nil
	}

	if f.Not != nil {
		skip, err := f.Not.SkipEvent(ctx, data, buf)
		if err != nil {
			return false, err
		}
		return !skip, nil
	}

	if f.In != nil {
		return f.In.SkipEvent(ctx, data, buf)
	}

	return false, nil
}

var (
	// findArray is a regex to find all array values in a string
	// e.g. [1, 2, 3] or ["a", "b", "c"]
	// it will skip prefix and suffix non array values, e.g. "foo[1, 2, 3]bar" will return [1, 2, 3]
	findArray                         = regexp.MustCompile(`\[(.*?)\]`)
	InvalidSubscriptionFilterTemplate = fmt.Errorf("invalid subscription filter template")
)

func (f *SubscriptionFieldFilter) SkipEvent(ctx *Context, data []byte, buf *bytes.Buffer) (bool, error) {
	if f == nil {
		return false, nil
	}

	expected, _, _, _err := jsonparser.Get(data, f.FieldPath...)
	if _err != nil {
		return true, nil
	}

	for i := range f.Values {
		buf.Reset()
		err := f.Values[i].Render(ctx, nil, buf)
		if err != nil {
			return false, err
		}
		actual := buf.Bytes()
		// cheap pre-check to see if we can skip the more expensive array check
		if !bytes.Contains(actual, literal.LBRACK) || !bytes.Contains(actual, literal.RBRACK) {
			if bytes.Equal(expected, actual) {
				return false, nil
			}
		}
		// check if the actual value contains an array, e.g. [1, 2, 3] or ["a", "b", "c"]
		// if it does, explode the array values into multiple values and compare each one
		// it's possible that the array is prefixed or suffixed with a non array value, e.g. "foo[1, 2, 3]bar"
		// so we need to check for that as well
		// start with a regex to find all array values
		// then check if the actual value contains the expected value
		matches := findArray.FindAllSubmatch(actual, -1)
		if matches == nil {
			if bytes.Equal(expected, actual) {
				return false, nil
			}
			continue
		}
		if len(matches) != 1 || len(matches[0]) != 2 {
			return false, InvalidSubscriptionFilterTemplate
		}
		arrayValue := matches[0][0]
		arrayMatch := false
		_, _ = jsonparser.ArrayEach(arrayValue, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			replaced := bytes.Replace(actual, matches[0][0], value, 1)
			if bytes.Equal(expected, replaced) {
				arrayMatch = true
			}
		})
		if arrayMatch {
			return false, nil
		}
	}

	return true, nil
}
