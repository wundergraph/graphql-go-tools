//go:generate go-enum -f=$GOFILE

package document

import (
	"strings"
)

// DirectiveLocation as specified in:
// http://facebook.github.io/graphql/draft/#DirectiveLocations
/*
ENUM(
QUERY
MUTATION
SUBSCRIPTION
FIELD
FRAGMENT_DEFINITION
FRAGMENT_SPREAD
INLINE_FRAGMENT
SCHEMA
SCALAR
OBJECT
FIELD_DEFINITION
ARGUMENT_DEFINITION
INTERFACE
UNION
ENUM
ENUM_VALUE
INPUT_OBJECT
INPUT_FIELD_DEFINITION
)
*/
type DirectiveLocation int

// DirectiveLocations is the plural of DirectiveLocation
type DirectiveLocations []DirectiveLocation

func (d DirectiveLocations) String() string {
	builder := strings.Builder{}
	builder.WriteString("[")
	for i, location := range d {
		builder.WriteString(location.String())
		if i < len(d)-1 {
			builder.WriteString(",")
		}
	}
	builder.WriteString("]")
	return builder.String()
}
