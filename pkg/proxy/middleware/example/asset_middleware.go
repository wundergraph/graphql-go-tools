package example

import (
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/runes"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"strings"
)

type AssetUrlMiddleware struct {
}

func (a *AssetUrlMiddleware) OnRequest(l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) {

	// get the required names (int)
	// if they don't exist in the type system definition we'd receive '-1' which indicates the literal doesn't exist
	assetName := l.ByteSliceName([]byte("Asset"))
	urlName := l.ByteSliceName([]byte("url"))
	handleName := l.ByteSliceName([]byte("handle"))

	// handle gracefully/error logging
	if assetName == -1 || urlName == -1 || handleName == -1 {
		return
	}

	field := document.Field{
		Name:         handleName,
		DirectiveSet: -1,
		ArgumentSet:  -1,
		SelectionSet: -1,
	}

	handleFieldRef := mod.PutField(field) // add the newly introduced field to the ast, we might cache such operations

	fields := w.FieldsIterable()
	for fields.Next() {
		field, fieldRef, parent := fields.Value()
		if field.Name != urlName { // find all fields in the ast named 'url'
			continue
		}
		setRef := w.Node(parent).Ref
		set := l.SelectionSet(setRef)
		setTypeName := w.SelectionSetTypeName(set, parent)
		if setTypeName != assetName { // verify if field 'url' sits inside an Asset type
			continue
		}
		mod.DeleteFieldFromSelectionSet(fieldRef, setRef) // delete the field on the selectionSet
	}

	sets := w.SelectionSetIterable()
	for sets.Next() {

		set, _, setRef, parent := sets.Value()
		typeName := w.SelectionSetTypeName(set, parent)
		if typeName != assetName { // find all selectionSets belonging to type Asset
			continue
		}

		mod.AppendFieldToSelectionSet(handleFieldRef, setRef) // append the handler field
	}

	return
}

func (a *AssetUrlMiddleware) OnResponse(response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {

	jsonObject, err := gabs.ParseJSON(*response)
	if err != nil {
		return err
	}

	w.SetLookup(l)
	w.WalkExecutable()

	assetName := l.ByteSliceName([]byte("Asset"))
	handleName := l.ByteSliceName([]byte("handle"))

	if assetName == -1 || handleName == -1 {
		return nil
	}

	fields := w.FieldsIterable()
	for fields.Next() {
		field, _, parent := fields.Value()
		if field.Name != handleName { // find all fields in the ast named 'url'
			continue
		}
		setRef := w.Node(parent).Ref
		set := l.SelectionSet(setRef)
		setTypeName := w.SelectionSetTypeName(set, parent)
		if setTypeName != assetName { // verify if field 'url' sits inside an Asset type
			continue
		}

		path := w.FieldPath(parent)
		var builder strings.Builder
		builder.WriteString("data")

		for i := range path {
			builder.WriteRune(runes.DOT)
			builder.Write(l.CachedName(path[len(path)-1-i]))
		}

		children, err := jsonObject.Path(builder.String()).Children()
		if err != nil {
			return err
		}

		for _, child := range children {
			handle := child.Path("handle").Data().(string)
			err = child.DeleteP("handle")
			_, err = child.Set(fmt.Sprintf("https://media.graphcms.com//%s", handle), "url")
		}
	}

	*response = jsonObject.Bytes()

	return err
}
