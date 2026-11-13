package plan

// A concrete type could be a part of multiple interface objects within the same subgraph, e.g.
//
//	type Alpha @key(fields: "id") @interfaceObject { id: ID! alphaField: String! }
//	type Beta @key(fields: "id") @interfaceObject { id: ID! betaField: String! }
//
// where both interface objects list the concrete type Thing.
// In this case the interface object used for an entity fetch has to be chosen
// by the requested field instead of the configuration order.

// InterfaceObjectNameForType returns the interface object to use for the concrete typeName
// when there is no field to disambiguate with.
// For a concrete type belonging to multiple interface objects the first configured one is returned.
func (d *DataSourceMetadata) InterfaceObjectNameForType(typeName string) (string, bool) {
	names := d.InterfaceObjectNamesForConcreteType(typeName)
	if len(names) == 0 {
		return "", false
	}

	return names[0], true
}

// InterfaceObjectNameForTypeField returns the interface object to use to query fieldName
// on the concrete typeName.
// When exactly one of the interface objects containing the concrete type defines the field,
// this interface object is returned, otherwise the first configured one.
func (d *DataSourceMetadata) InterfaceObjectNameForTypeField(typeName, fieldName string) (string, bool) {
	if name, ok := d.UnambiguousInterfaceObjectNameForTypeField(typeName, fieldName); ok {
		return name, true
	}

	return d.InterfaceObjectNameForType(typeName)
}

// UnambiguousInterfaceObjectNameForTypeField returns an interface object only when the concrete
// typeName belongs to multiple interface objects and exactly one of them defines fieldName.
// Such a field could only be resolved via this interface object.
func (d *DataSourceMetadata) UnambiguousInterfaceObjectNameForTypeField(typeName, fieldName string) (string, bool) {
	names := d.InterfaceObjectNamesForConcreteType(typeName)
	if len(names) < 2 {
		return "", false
	}

	var owner string
	for _, name := range names {
		if !d.hasNodeWithField(name, fieldName) {
			continue
		}
		if owner != "" {
			return "", false
		}
		owner = name
	}

	return owner, owner != ""
}

func (d *DataSourceMetadata) hasNodeWithField(typeName, fieldName string) bool {
	return d.HasRootNode(typeName, fieldName) ||
		d.HasChildNode(typeName, fieldName) ||
		d.HasExternalRootNode(typeName, fieldName) ||
		d.HasExternalChildNode(typeName, fieldName)
}
