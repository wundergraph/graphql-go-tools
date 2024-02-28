package plan

type FederationMetaData struct {
	Keys             FederationFieldConfigurations
	Requires         FederationFieldConfigurations
	Provides         FederationFieldConfigurations
	EntityInterfaces []EntityInterfaceConfiguration
	InterfaceObjects []EntityInterfaceConfiguration
}

type EntityInterfaceConfiguration struct {
	InterfaceTypeName string
	ConcreteTypeNames []string
}

type FederationFieldConfiguration struct {
	TypeName              string
	FieldName             string
	SelectionSet          string
	DisableEntityResolver bool // applicable only for the keys. If true it means that the given entity could not be resolved by this key.
}

type FederationFieldConfigurations []FederationFieldConfiguration

func (f FederationFieldConfigurations) FilterByTypeAndResolvability(typeName string, skipUnresovable bool) (out []FederationFieldConfiguration) {
	for i := range f {
		if f[i].TypeName != typeName || f[i].FieldName != "" {
			continue
		}
		if skipUnresovable && f[i].DisableEntityResolver {
			continue
		}
		out = append(out, f[i])
	}
	return out
}

func (f FederationFieldConfigurations) UniqueTypes() (out []string) {
	seen := map[string]struct{}{}
	for i := range f {
		seen[f[i].TypeName] = struct{}{}
	}

	for k := range seen {
		out = append(out, k)
	}
	return out
}

func (f FederationFieldConfigurations) FilterByTypeAndField(typeName, fieldName string) (out []FederationFieldConfiguration) {
	for i := range f {
		if f[i].TypeName != typeName || f[i].FieldName != fieldName {
			continue
		}
		out = append(out, f[i])
	}
	return out
}

func (f FederationFieldConfigurations) HasSelectionSet(typeName, fieldName, selectionSet string) (ok bool) {
	for i := range f {
		if typeName == f[i].TypeName &&
			fieldName == f[i].FieldName &&
			selectionSet == f[i].SelectionSet {
			return true
		}
	}
	return false
}

func appendRequiredFieldsConfigurationIfNotPresent(configs FederationFieldConfigurations, config FederationFieldConfiguration) (cfgs FederationFieldConfigurations, added bool) {
	ok := configs.HasSelectionSet(config.TypeName, config.FieldName, config.SelectionSet)
	if !ok {
		return append(configs, config), true
	}

	return configs, false
}
