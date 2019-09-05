//go:generate go-enum -f=$GOFILE

package document

// ValueType differentiates the different types of values
/*
ENUM(
DefaultNull
VariableValue
Int
Float
String
Boolean
Null
Enum
List
Object
)
*/
type ValueType int
