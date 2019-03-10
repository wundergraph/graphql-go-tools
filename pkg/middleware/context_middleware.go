package middleware

import (
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
)

type ContextMiddleware struct{}

func (a *ContextMiddleware) OnResponse(context context.Context, response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {
	return nil
}

type ContextRewriteConfig struct {
	fieldName               int
	argumentName            int
	argumentValueContextKey document.ByteSlice
}

func (a *ContextMiddleware) OnRequest(context context.Context, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {

	w.SetLookup(l)
	w.WalkTypeSystemDefinition()

	addArgumentFromContextDirectiveName, _, err := mod.PutLiteralBytes([]byte("addArgumentFromContext"))
	if err != nil {
		return err
	}

	nameLiteral, _, err := mod.PutLiteralBytes([]byte("name"))
	if err != nil {
		return err
	}

	contextKeyLiteral, _, err := mod.PutLiteralBytes([]byte("contextKey"))
	if err != nil {
		return err
	}

	typeNamesAndFieldNamesWithDirective := make(map[int][]ContextRewriteConfig)

	directiveSets := w.DirectiveSetIterable()
	for directiveSets.Next() {
		set, parent := directiveSets.Value()
		directives := l.DirectiveIterable(set)
		for directives.Next() {
			directive, _ := directives.Value()
			if directive.Name != addArgumentFromContextDirectiveName {
				continue
			}

			//fmt.Println("found directive")

			field := w.Node(parent)
			if field.Kind != lookup.FIELD_DEFINITION {
				continue
			}

			fieldDefintion := l.FieldDefinition(field.Ref)

			//fmt.Println("parent is field")

			fieldType := w.Node(field.Parent)
			if fieldType.Kind != lookup.OBJECT_TYPE_DEFINITION {
				continue
			}

			//fmt.Println("fieldType is object type definition")

			objectTypeDefinition := l.ObjectTypeDefinition(fieldType.Ref)

			rewriteConfig := ContextRewriteConfig{
				fieldName: fieldDefintion.Name,
			}

			argSet := l.ArgumentSet(directive.ArgumentSet)
			args := l.ArgumentsIterable(argSet)
			for args.Next() {
				arg, _ := args.Value()
				if arg.Name == nameLiteral {
					value := l.Value(arg.Value)
					raw := l.ByteSlice(value.Raw)

					//raw = bytes.Replace(raw,literal.QUOTE,nil,-1)

					argName, _, err := mod.PutLiteralBytes(raw)
					if err != nil {
						return err
					}

					rewriteConfig.argumentName = argName
				} else if arg.Name == contextKeyLiteral {
					value := l.Value(arg.Value)
					raw := l.ByteSlice(value.Raw)

					//raw = bytes.Replace(raw,literal.QUOTE,nil,-1)

					rewriteConfig.argumentValueContextKey = raw
				}
			}

			typeNamesAndFieldNamesWithDirective[objectTypeDefinition.Name] = append(typeNamesAndFieldNamesWithDirective[objectTypeDefinition.Name], rewriteConfig)
		}
	}

	//fmt.Printf("directive on fields: %+v\n", typeNamesAndFieldNamesWithDirective)

	w.SetLookup(l)
	w.WalkExecutable()

	selectionSets := w.SelectionSetIterable()
	for selectionSets.Next() {
		set, _, _, parent := selectionSets.Value()
		typeName := w.SelectionSetTypeName(set, parent)
		fieldsWithDirective, ok := typeNamesAndFieldNamesWithDirective[typeName]
		if !ok {
			continue
		}

		//fmt.Printf("fieldsWithDirective: %+v\n", fieldsWithDirective)

		fields := l.SelectionSetCollectedFields(set, typeName)
		for fields.Next() {
			fieldRef, field := fields.Value()
			for _, i := range fieldsWithDirective {
				if i.fieldName == field.Name {
					//fmt.Printf("must merge args into: %d\n", field.ArgumentSet)

					argumentValue := context.Value(string(i.argumentValueContextKey)).([]byte)
					//fmt.Printf("argumentValue: %s\n", string(argumentValue))

					argNameRef, argByteSliceRef, err := mod.PutLiteralBytes(argumentValue)
					if err != nil {
						return err
					}

					val := document.Value{
						ValueType: document.ValueTypeString,
						Raw:       argByteSliceRef,
						Reference: argNameRef,
					}

					valueRef := mod.PutValue(val)

					arg := document.Argument{
						Name:  i.argumentName,
						Value: valueRef,
					}

					argRef := mod.PutArgument(arg)

					mod.MergeArgIntoFieldArguments(argRef, fieldRef)
				}
			}
		}
	}

	return nil
}
