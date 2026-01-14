package resolve

import "errors"

const (
	locationsField = "locations"
)

var (
	lBrace                 = []byte("{")
	rBrace                 = []byte("}")
	lBrack                 = []byte("[")
	rBrack                 = []byte("]")
	comma                  = []byte(",")
	pipe                   = []byte("|")
	dot                    = []byte(".")
	colon                  = []byte(":")
	quote                  = []byte("\"")
	null                   = []byte("null")
	literalData            = []byte("data")
	literalErrors          = []byte("errors")
	literalMessage         = []byte("message")
	literalLocations       = []byte(locationsField)
	literalPath            = []byte("path")
	literalExtensions      = []byte("extensions")
	literalTrace           = []byte("trace")
	literalQueryPlan       = []byte("queryPlan")
	literalValueCompletion = []byte("valueCompletion")
	literalRateLimit       = []byte("rateLimit")
	literalAuthorization   = []byte("authorization")

	emptyObject = []byte("{}")
)

var (
	errHeaderPathInvalid = errors.New("invalid header path: header variables must be of this format: .request.header.{{ key }} ")
	ErrUnableToResolve   = errors.New("unable to resolve operation")
)
