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
	literalTrue            = []byte("true")
	literalFalse           = []byte("false")
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
	literalIncremental     = []byte("incremental")
	literalHasNext         = []byte("hasNext")
	literalLabel           = []byte("label")
	literalPending         = []byte("pending")
	literalCompleted       = []byte("completed")
	literalId              = []byte("id")
	literalSubPath         = []byte("subPath")

	emptyObject = []byte("{}")
)

var (
	errHeaderPathInvalid = errors.New("invalid header path: header variables must be of this format: .request.header.{{ key }} ")
	ErrUnableToResolve   = errors.New("unable to resolve operation")
)
