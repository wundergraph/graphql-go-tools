package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

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

// FieldCoordinate contains coordinates of a field in a type
// TODO: rename to FieldCoordinates
type FieldCoordinate struct {
	TypeName  string `json:"type_name"`
	FieldName string `json:"field_name"`
}

func (f FieldCoordinate) String() string {
	return fmt.Sprintf("%s.%s", f.TypeName, f.FieldName)
}

// parseSelectionSet parses the selection set and stores the parsed AST in parsedSelectionSet.
// should have pointer receiver to preserve the value
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

// String - implements fmt.Stringer
// NOTE: do not change to pointer receiver, it won't work for not pointer values
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

func (f *FederationFieldConfigurations) HasArgumentConflictWith(configs []FederationFieldConfiguration) bool {
	for i := range *f {
		for j := range configs {
			if requiredFieldArgumentConflict((*f)[i], configs[j]) {
				return true
			}
		}
	}

	return false
}

func requiredFieldArgumentConflict(left, right FederationFieldConfiguration) bool {
	if left.TypeName != right.TypeName {
		return false
	}

	leftFields, ok := requiredFieldArgumentsByPath(left)
	if !ok {
		return false
	}
	rightFields, ok := requiredFieldArgumentsByPath(right)
	if !ok {
		return false
	}

	for path, leftArguments := range leftFields {
		rightArguments, exists := rightFields[path]
		if exists && leftArguments != rightArguments {
			return true
		}
	}

	return false
}

func requiredFieldArgumentsByPath(config FederationFieldConfiguration) (map[string]string, bool) {
	if err := config.parseSelectionSet(); err != nil {
		return nil, false
	}
	if len(config.parsedSelectionSet.FragmentDefinitions) == 0 {
		return nil, false
	}

	out := make(map[string]string)
	collectRequiredFieldArguments(config.parsedSelectionSet, config.parsedSelectionSet.FragmentDefinitions[0].SelectionSet, nil, out)

	return out, true
}

func collectRequiredFieldArguments(doc *ast.Document, selectionSetRef int, path []string, out map[string]string) {
	for _, selectionRef := range doc.SelectionSets[selectionSetRef].SelectionRefs {
		selection := doc.Selections[selectionRef]
		switch selection.Kind {
		case ast.SelectionKindField:
			fieldRef := selection.Ref
			fieldPath := append(path, doc.FieldNameString(fieldRef))
			out[strings.Join(fieldPath, ".")] = requiredFieldArgumentSignature(doc, fieldRef)
			if doc.FieldHasSelections(fieldRef) {
				collectRequiredFieldArguments(doc, doc.Fields[fieldRef].SelectionSet, fieldPath, out)
			}
		case ast.SelectionKindInlineFragment:
			inlineFragmentRef := selection.Ref
			fragmentPath := append(path, "... on "+doc.InlineFragmentTypeConditionNameString(inlineFragmentRef))
			if doc.InlineFragments[inlineFragmentRef].HasSelections {
				collectRequiredFieldArguments(doc, doc.InlineFragments[inlineFragmentRef].SelectionSet, fragmentPath, out)
			}
		}
	}
}

func requiredFieldArgumentSignature(doc *ast.Document, fieldRef int) string {
	if !doc.FieldHasArguments(fieldRef) {
		return ""
	}

	args := make([]string, 0, len(doc.FieldArguments(fieldRef)))
	for _, argRef := range doc.FieldArguments(fieldRef) {
		var buf bytes.Buffer
		_ = doc.PrintArgument(argRef, &buf)
		args = append(args, buf.String())
	}
	slices.Sort(args)

	return strings.Join(args, ",")
}
