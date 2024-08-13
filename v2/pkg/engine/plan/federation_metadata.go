package plan

type FederationMetaData struct {
	Keys             FederationFieldConfigurations
	Requires         FederationFieldConfigurations
	Provides         FederationFieldConfigurations
	EntityInterfaces []EntityInterfaceConfiguration
	InterfaceObjects []EntityInterfaceConfiguration
}

type FederationInfo interface {
	HasKeyRequirement(typeName, requiresFields string) bool
	RequiredFieldsByKey(typeName string) []FederationFieldConfiguration
	RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool)
	HasEntity(typeName string) bool
}

func (d *FederationMetaData) HasKeyRequirement(typeName, requiresFields string) bool {
	return d.Keys.HasSelectionSet(typeName, "", requiresFields)
}

func (d *FederationMetaData) RequiredFieldsByKey(typeName string) []FederationFieldConfiguration {
	return d.Keys.FilterByTypeAndResolvability(typeName, true)
}

func (d *FederationMetaData) HasEntity(typeName string) bool {
	return len(d.Keys.FilterByTypeAndResolvability(typeName, false)) > 0
}

func (d *FederationMetaData) RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool) {
	return d.Requires.FirstByTypeAndField(typeName, fieldName)
}

type EntityInterfaceConfiguration struct {
	InterfaceTypeName string
	ConcreteTypeNames []string
}

type FederationFieldConfiguration struct {
	// TypeName is the name of the Entity the Fragment is for
	TypeName string
	// FieldName is empty for key requirements, otherwise it is the name of the field that requires the given
	FieldName string
	// SelectionSet is the selection set that is required for the given field (keys, requires, provides)
	SelectionSet          string
	DisableEntityResolver bool // applicable only for the keys. If true it means that the given entity could not be resolved by this key.
}

type FederationFieldConfigurations []FederationFieldConfiguration

func (f *FederationFieldConfigurations) FilterByTypeAndResolvability(typeName string, skipUnresovable bool) (out []FederationFieldConfiguration) {
	for i := range *f {
		if (*f)[i].TypeName != typeName || (*f)[i].FieldName != "" {
			continue
		}
		if skipUnresovable && (*f)[i].DisableEntityResolver {
			continue
		}
		out = append(out, (*f)[i])
	}
	return out
}

func (f *FederationFieldConfigurations) UniqueTypes() (out []string) {
	seen := map[string]struct{}{}
	for i := range *f {
		seen[(*f)[i].TypeName] = struct{}{}
	}

	for k := range seen {
		out = append(out, k)
	}
	return out
}

func (f *FederationFieldConfigurations) FirstByTypeAndField(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool) {
	for i := range *f {
		if (*f)[i].TypeName == typeName && (*f)[i].FieldName == fieldName {
			return (*f)[i], true
		}
	}
	return FederationFieldConfiguration{}, false
}

func (f *FederationFieldConfigurations) HasSelectionSet(typeName, fieldName, selectionSet string) bool {
	for i := range *f {
		if typeName == (*f)[i].TypeName &&
			fieldName == (*f)[i].FieldName &&
			selectionSet == (*f)[i].SelectionSet {
			return true
		}
	}
	return false
}

func (f *FederationFieldConfigurations) AppendIfNotPresent(config FederationFieldConfiguration) (added bool) {
	ok := f.HasSelectionSet(config.TypeName, config.FieldName, config.SelectionSet)
	if ok {
		return false
	}

	*f = append(*f, config)

	return true
}
