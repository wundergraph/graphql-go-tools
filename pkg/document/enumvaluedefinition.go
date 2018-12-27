package document

import (
	"bytes"
)

// EnumValueDefinition as specified in:
// http://facebook.github.io/graphql/draft/#EnumValueDefinition
type EnumValueDefinition struct {
	Description ByteSlice
	EnumValue   ByteSlice
	Directives  Directives
}

// ProperCaseVal returns the EnumValueDefinition's EnumValue
// as proper case string. example:
// NORTH => North
func (e EnumValueDefinition) ProperCaseVal() []byte {
	return bytes.Title(bytes.ToLower(e.EnumValue))
}
