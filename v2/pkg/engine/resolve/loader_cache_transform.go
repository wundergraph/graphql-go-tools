package resolve

import "github.com/wundergraph/astjson"

// structuralCopyNormalized performs the L2 write copy: alias keys are renamed to
// schema keys and unlisted fields are projected out.
func (l *Loader) structuralCopyNormalized(v *astjson.Value, provides *Object) *astjson.Value {
	if provides == nil {
		return astjson.StructuralCopy(l.jsonArena, v)
	}
	return astjson.StructuralCopyWithTransform(l.jsonArena, v, buildNormalizeTransform(provides, false))
}

// structuralCopyDenormalized performs the L2 read copy: schema keys are renamed
// back to alias keys and unlisted fields are projected out.
func (l *Loader) structuralCopyDenormalized(v *astjson.Value, provides *Object) *astjson.Value {
	if provides == nil {
		return astjson.StructuralCopy(l.jsonArena, v)
	}
	return astjson.StructuralCopyWithTransform(l.jsonArena, v, buildDenormalizeTransform(provides, false))
}

// structuralCopyNormalizedPassthrough performs the L1 write copy: alias keys are
// renamed to schema keys while unlisted fields are kept.
func (l *Loader) structuralCopyNormalizedPassthrough(v *astjson.Value, provides *Object) *astjson.Value {
	if provides == nil {
		return astjson.StructuralCopy(l.jsonArena, v)
	}
	return astjson.StructuralCopyWithTransform(l.jsonArena, v, buildNormalizeTransform(provides, true))
}

// structuralCopyDenormalizedPassthrough performs the L1 read copy: schema keys
// are renamed back to alias keys while unlisted fields are kept.
func (l *Loader) structuralCopyDenormalizedPassthrough(v *astjson.Value, provides *Object) *astjson.Value {
	if provides == nil {
		return astjson.StructuralCopy(l.jsonArena, v)
	}
	return astjson.StructuralCopyWithTransform(l.jsonArena, v, buildDenormalizeTransform(provides, true))
}

// buildNormalizeTransform builds an alias-to-schema transform for cache writes.
// With passthrough false it is the L2 projection shape; with passthrough true it
// is the L1 keep-extra-fields shape.
func buildNormalizeTransform(provides *Object, passthrough bool) *astjson.Transform {
	if provides == nil {
		return nil
	}

	transform := &astjson.Transform{
		Entries:     make([]astjson.TransformEntry, 0, len(provides.Fields)),
		Passthrough: passthrough,
	}
	for _, field := range provides.Fields {
		if field == nil {
			continue
		}
		transform.Entries = append(transform.Entries, astjson.TransformEntry{
			InputKey:  fieldAliasName(field),
			OutputKey: fieldSchemaName(field),
			Child:     buildNormalizeChildTransform(field.Value, passthrough),
		})
	}
	return transform
}

// buildDenormalizeTransform builds a schema-to-alias transform for cache reads.
// With passthrough false it is the L2 projection shape; with passthrough true it
// is the L1 keep-extra-fields shape.
func buildDenormalizeTransform(provides *Object, passthrough bool) *astjson.Transform {
	if provides == nil {
		return nil
	}

	transform := &astjson.Transform{
		Entries:     make([]astjson.TransformEntry, 0, len(provides.Fields)),
		Passthrough: passthrough,
	}
	for _, field := range provides.Fields {
		if field == nil {
			continue
		}
		transform.Entries = append(transform.Entries, astjson.TransformEntry{
			InputKey:  fieldSchemaName(field),
			OutputKey: fieldAliasName(field),
			Child:     buildDenormalizeChildTransform(field.Value, passthrough),
		})
	}
	return transform
}

// buildNormalizeChildTransform builds nested alias-to-schema transforms for the
// L1 and L2 write helpers.
func buildNormalizeChildTransform(node Node, passthrough bool) *astjson.Transform {
	switch value := node.(type) {
	case *Object:
		return buildNormalizeTransform(value, passthrough)
	case *Array:
		itemObject, ok := value.Item.(*Object)
		if !ok {
			return nil
		}
		return &astjson.Transform{
			ArrayItem:   buildNormalizeTransform(itemObject, passthrough),
			Passthrough: passthrough,
		}
	default:
		return nil
	}
}

// buildDenormalizeChildTransform builds nested schema-to-alias transforms for
// the L1 and L2 read helpers.
func buildDenormalizeChildTransform(node Node, passthrough bool) *astjson.Transform {
	switch value := node.(type) {
	case *Object:
		return buildDenormalizeTransform(value, passthrough)
	case *Array:
		itemObject, ok := value.Item.(*Object)
		if !ok {
			return nil
		}
		return &astjson.Transform{
			ArrayItem:   buildDenormalizeTransform(itemObject, passthrough),
			Passthrough: passthrough,
		}
	default:
		return nil
	}
}

// fieldAliasName returns the response key used by denormalized cache reads.
func fieldAliasName(field *Field) string {
	return string(field.Name)
}

// fieldSchemaName returns the schema key used by normalized cache writes.
func fieldSchemaName(field *Field) string {
	if len(field.OriginalName) > 0 {
		return string(field.OriginalName)
	}
	return string(field.Name)
}
