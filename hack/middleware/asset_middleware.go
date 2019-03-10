package middleware_wip

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/runes"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"strconv"
	"strings"
)

type AssetUrlMiddleware struct {
}

func (a *AssetUrlMiddleware) OnRequest(context context.Context, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {

	w.SetLookup(l)
	w.WalkExecutable()

	// get the required names (int)
	// if they don't exist in the type system definition we'd receive '-1' which indicates the literal doesn't exist
	assetName := l.ByteSliceName([]byte("Asset"))
	urlName := l.ByteSliceName([]byte("url"))
	handleName := l.ByteSliceName([]byte("handle"))

	// handle gracefully/error logging
	if assetName == -1 || urlName == -1 || handleName == -1 {
		return nil
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

	return nil
}

func (a *AssetUrlMiddleware) OnResponse(context context.Context, response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) (err error) {

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

		path := w.FieldPath(parent) // get the field path from the ast

		var builder strings.Builder // build the path string (e.g. 'data.assets'
		builder.WriteString("data")
		for i := range path {
			builder.WriteRune(runes.DOT)
			builder.Write(l.CachedName(path[len(path)-1-i]))
		}

		builder.WriteString(".#.handle")

		strPath := builder.String()

		js := gjson.GetBytes(*response, strPath)

		for i, handle := range js.Array() {
			id := handle.String()
			handlePath := strings.Replace(strPath, "#", strconv.Itoa(i), 1)
			url := fmt.Sprintf("https://media.graphcms.com//%s", id)
			*response, err = sjson.SetBytesOptions(*response, handlePath, url, &sjson.Options{Optimistic: true, ReplaceInPlace: true})
		}

		*response = bytes.Replace(*response, []byte(`"handle"`), []byte(`"url"`), -1)

		/*children, err := jsonObject.Path(builder.String()).Children() // get the assets children
		if err != nil {
			return err
		}

		for _, child := range children {
			handle := child.Path("handle").Data().(string)                                   // extract the handle value
			err = child.DeleteP("handle")                                                    // delete the handle value
			_, err = child.Set(fmt.Sprintf("https://media.graphcms.com//%s", handle), "url") // set the formatted url value
		}*/
	}

	// *response = jsonObject.Bytes() // overwrite the response with the updated fields

	return
}
