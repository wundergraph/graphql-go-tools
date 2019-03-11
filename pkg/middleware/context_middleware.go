package middleware

import (
	"context"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
)

/*
directive @addArgumentFromContext(
	name: String!
	contextKey: String!
) on FIELD_DEFINITION
*/

/*
ContextMiddleware does rewrite graphql requests based on schema annotations using a provided context object as input

example schema:

type Query {
	documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
}

given there's an object with key "user" and value "jsmith@example.org" in the context

original Request:

query myDocuments {
	documents {
		sensitiveInformation
	}
}

Request after rewriting:

query myDocuments {
	documents(user: "jsmith@example.org") {
		sensitiveInformation
	}
}

*/
type ContextMiddleware struct {
}

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

	fields := w.FieldsContainingDirectiveIterator(addArgumentFromContextDirectiveName)
	for fields.Next() {
		fieldRef, objectTypeDefinitionRef, directiveRef := fields.Value()

		directive := l.Directive(directiveRef)
		fieldDefinition := l.FieldDefinition(fieldRef)
		objectTypeDefinition := l.ObjectTypeDefinition(objectTypeDefinitionRef)

		rewriteConfig := ContextRewriteConfig{
			fieldName: fieldDefinition.Name,
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
