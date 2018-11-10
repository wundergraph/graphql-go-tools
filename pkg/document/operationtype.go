//go:generate go-enum -f=$GOFILE

package document

// OperationType is the type of the Operation
/*
ENUM(
query
mutation
subscription
)
*/
type OperationType int
