package plan

import (
	"encoding/json"
	"slices"
)

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
	HasInterfaceObject(typeName string) bool
	HasEntityInterface(typeName string) bool
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

func (d *FederationMetaData) HasInterfaceObject(typeName string) bool {
	return slices.ContainsFunc(d.InterfaceObjects, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
		return slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) || interfaceObjCfg.InterfaceTypeName == typeName
	})
}

func (d *FederationMetaData) HasEntityInterface(typeName string) bool {
	return slices.ContainsFunc(d.EntityInterfaces, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
		return slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) || interfaceObjCfg.InterfaceTypeName == typeName
	})
}

type EntityInterfaceConfiguration struct {
	InterfaceTypeName string
	ConcreteTypeNames []string
}

type FederationFieldConfiguration struct {
	TypeName              string `json:"type_name"`            // TypeName is the name of the Entity the Fragment is for
	FieldName             string `json:"field_name,omitempty"` // FieldName is empty for key requirements, otherwise, it is the name of the field that has requires or provides directive
	SelectionSet          string `json:"selection_set"`        // SelectionSet is the selection set that is required for the given field (keys, requires, provides)
	DisableEntityResolver bool   `json:"-"`                    // applicable only for the keys. If true it means that the given entity could not be resolved by this key.
}

func (f FederationFieldConfiguration) String() string {
	b, _ := json.Marshal(f)
	return string(b)
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
