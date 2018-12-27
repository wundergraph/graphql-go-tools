package document

import "bytes"

// UnionTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#UnionTypeDefinition
type UnionTypeDefinition struct {
	Description      ByteSlice
	Name             ByteSlice
	UnionMemberTypes UnionMemberTypes
	Directives       Directives
}

// GroupingFuncName returns a name to name a function after. Example:
// "Direction" => "isDirection"
func (u UnionTypeDefinition) GroupingFuncName() []byte {
	return append([]byte("Is"), u.Name...)
}

// HasMemberType returns true if a member with the given name is contained
func (u UnionTypeDefinition) HasMemberType(name []byte) bool {
	for _, unionMemberType := range u.UnionMemberTypes {
		if bytes.Equal(unionMemberType, name) {
			return true
		}
	}

	return false
}

// UnionMemberTypes as specified in:
// http://facebook.github.io/graphql/draft/#UnionMemberTypes
type UnionMemberTypes []ByteSlice

// UnionTypeDefinitions is the plural of UnionTypeDefinition
type UnionTypeDefinitions []UnionTypeDefinition

// GetByName returns the UnionTypeDefinition by $name if it is contained
func (u UnionTypeDefinitions) GetByName(name []byte) *UnionTypeDefinition {
	for _, definition := range u {
		if bytes.Equal(definition.Name, name) {
			return &definition
		}
	}

	return nil
}
