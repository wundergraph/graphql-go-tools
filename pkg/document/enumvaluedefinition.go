package document

import (
	"strings"
)

// EnumValueDefinition as specified in:
// http://facebook.github.io/graphql/draft/#EnumValueDefinition
type EnumValueDefinition struct {
	Description string
	EnumValue   string
	Directives  []int
}

// ProperCaseVal returns the EnumValueDefinition's EnumValue
// as proper case string. example:
// NORTH => North
func (e EnumValueDefinition) ProperCaseVal() string {
	return strings.Title(strings.ToLower(e.EnumValue))
}
