package plan

import (
	"encoding/json"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type FederationMetaData struct {
	Keys             FederationFieldConfigurations
	Requires         FederationFieldConfigurations
	Provides         FederationFieldConfigurations
	EntityInterfaces []EntityInterfaceConfiguration
	InterfaceObjects []EntityInterfaceConfiguration

	entityTypeNames map[string]struct{}
}

type FederationInfo interface {
	HasKeyRequirement(typeName, requiresFields string) bool
	RequiredFieldsByKey(typeName string) []FederationFieldConfiguration
	RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool)
	HasEntity(typeName string) bool
	HasInterfaceObject(typeName string) bool
	HasEntityInterface(typeName string) bool
	EntityInterfaceNames() []string
}

func (d *FederationMetaData) HasKeyRequirement(typeName, requiresFields string) bool {
	return d.Keys.HasSelectionSet(typeName, "", requiresFields)
}

func (d *FederationMetaData) RequiredFieldsByKey(typeName string) []FederationFieldConfiguration {
	return d.Keys.FilterByTypeAndResolvability(typeName, true)
}

func (d *FederationMetaData) HasEntity(typeName string) bool {
	_, ok := d.entityTypeNames[typeName]
	return ok
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

func (d *FederationMetaData) EntityInterfaceNames() (out []string) {
	if len(d.EntityInterfaces) == 0 {
		return nil
	}

	for i := range d.EntityInterfaces {
		out = append(out, d.EntityInterfaces[i].InterfaceTypeName)
	}

	return out
}

type EntityInterfaceConfiguration struct {
	InterfaceTypeName string
	ConcreteTypeNames []string
}

type FederationFieldConfiguration struct {
	TypeName              string         `json:"type_name"`            // TypeName is the name of the Entity the Fragment is for
	FieldName             string         `json:"field_name,omitempty"` // FieldName is empty for key requirements, otherwise, it is the name of the field that has requires or provides directive
	SelectionSet          string         `json:"selection_set"`        // SelectionSet is the selection set that is required for the given field (keys, requires, provides)
	DisableEntityResolver bool           `json:"-"`                    // applicable only for the keys. If true it means that the given entity could not be resolved by this key.
	Conditions            []KeyCondition `json:"conditions,omitempty"` // conditions stores coordinates under which we could use implicit key, while on other paths this key is not available

	parsedSelectionSet *ast.Document
	RemappedPaths      map[string]string
}

type KeyCondition struct {
	Coordinates []FieldCoordinate `json:"coordinates"`
	FieldPath   []string          `json:"field_path"`
}

type FieldCoordinate struct {
	TypeName  string `json:"type_name"`
	FieldName string `json:"field_name"`
}

func (f *FederationFieldConfiguration) parseSelectionSet() error {
	if f.parsedSelectionSet != nil {
		return nil
	}

	doc, report := RequiredFieldsFragment(f.TypeName, f.SelectionSet, false)
	if report.HasErrors() {
		return report
	}

	f.parsedSelectionSet = doc
	return nil
}

func (f *FederationFieldConfiguration) String() string {
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
