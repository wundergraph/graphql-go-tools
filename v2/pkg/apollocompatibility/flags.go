package apollocompatibility

type Flags struct {
	ReplaceInvalidVarError       bool
	ReplaceUndefinedOpFieldError bool
}

type ApolloRouterFlags struct {
	ReplaceInvalidVarError bool
	SubrequestHTTPErrror   bool
}
