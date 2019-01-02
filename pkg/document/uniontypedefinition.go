package document

// UnionTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#UnionTypeDefinition
type UnionTypeDefinition struct {
	Description      string
	Name             string
	UnionMemberTypes UnionMemberTypes
	Directives       Directives
}

// GroupingFuncName returns a name to name a function after. Example:
// "Direction" => "IsDirection"
func (u UnionTypeDefinition) GroupingFuncName() string {
	return "Is" + u.Name
}

// HasMemberType returns true if a member with the given name is contained
func (u UnionTypeDefinition) HasMemberType(name string) bool {
	for _, unionMemberType := range u.UnionMemberTypes {
		if unionMemberType == name {
			return true
		}
	}

	return false
}

// UnionMemberTypes as specified in:
// http://facebook.github.io/graphql/draft/#UnionMemberTypes
type UnionMemberTypes []string

// UnionTypeDefinitions is the plural of UnionTypeDefinition
type UnionTypeDefinitions []UnionTypeDefinition

// GetByName returns the UnionTypeDefinition by $name if it is contained
func (u UnionTypeDefinitions) GetByName(name string) *UnionTypeDefinition {
	for _, definition := range u {
		if definition.Name == name {
			return &definition
		}
	}

	return nil
}
