package resolve

import "errors"

var (
	lBrace            = []byte("{")
	rBrace            = []byte("}")
	lBrack            = []byte("[")
	rBrack            = []byte("]")
	comma             = []byte(",")
	colon             = []byte(":")
	quote             = []byte("\"")
	quotedComma       = []byte(`","`)
	null              = []byte("null")
	literalData       = []byte("data")
	literalTrue       = []byte("true")
	literalFalse      = []byte("false")
	literalErrors     = []byte("errors")
	literalMessage    = []byte("message")
	literalLocations  = []byte("locations")
	literalLine       = []byte("line")
	literalColumn     = []byte("column")
	literalPath       = []byte("path")
	literalExtensions = []byte("extensions")
	literalTrace      = []byte("trace")

	unableToResolveMsg = []byte("unable to resolve")
	emptyArray         = []byte("[]")
	emptyObject        = []byte("{}")
)

var (
	errNonNullableFieldValueIsNull = errors.New("non Nullable field value is null")
	errInvalidFieldValue           = errors.New("invalid field value")
	errTypeNameSkipped             = errors.New("skipped because of __typename condition")
	errHeaderPathInvalid           = errors.New("invalid header path: header variables must be of this format: .request.header.{{ key }} ")

	ErrUnableToResolve = errors.New("unable to resolve operation")
)

var (
	errorPaths = [][]string{
		{"message"},
		{"locations"},
		{"path"},
		{"extensions"},
	}
)

const (
	errorsMessagePathIndex    = 0
	errorsLocationsPathIndex  = 1
	errorsPathPathIndex       = 2
	errorsExtensionsPathIndex = 3
)
