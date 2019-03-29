package middleware

import (
	"fmt"
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

func (a *ContextMiddleware) OnResponse(userValues map[string][]byte, response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {
	return nil
}

type ContextRewriteConfig struct {
	fieldName               document.ByteSliceReference
	argumentName            document.ByteSliceReference
	argumentValueContextKey document.ByteSlice
}

func (a *ContextMiddleware) OnRequest(userValues map[string][]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {

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

	typeNamesAndFieldNamesWithDirective := make(map[string][]ContextRewriteConfig)

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
			if l.ByteSliceReferenceContentsEquals(arg.Name, nameLiteral) {
				value := l.Value(arg.Value)
				rewriteConfig.argumentName = value.Raw
			} else if l.ByteSliceReferenceContentsEquals(arg.Name, contextKeyLiteral) {
				value := l.Value(arg.Value)
				rewriteConfig.argumentValueContextKey = l.ByteSlice(value.Raw)
			}
		}

		typeNamesAndFieldNamesWithDirective[string(l.ByteSlice(objectTypeDefinition.Name))] = append(typeNamesAndFieldNamesWithDirective[string(l.ByteSlice(objectTypeDefinition.Name))], rewriteConfig)
	}

	w.SetLookup(l)
	w.WalkExecutable()

	selectionSets := w.SelectionSetIterable()
	for selectionSets.Next() {
		set, _, _, parent := selectionSets.Value()
		typeName := w.SelectionSetTypeName(set, parent)
		fieldsWithDirective, ok := typeNamesAndFieldNamesWithDirective[string(l.ByteSlice(typeName))]
		if !ok {
			continue
		}

		fields := l.SelectionSetCollectedFields(set, typeName)
		for fields.Next() {
			fieldRef, field := fields.Value()
			for _, i := range fieldsWithDirective {
				if l.ByteSliceReferenceContentsEquals(i.fieldName, field.Name) {

					argumentValue := userValues[string(i.argumentValueContextKey)]
					if argumentValue == nil {
						return fmt.Errorf("OnRequest: No value for key: %s (did you forget to configure setting the 'contextKeys' configuration which enables loading variables from the header into the context values?)\n", string(i.argumentValueContextKey))
					}

					argByteSliceRef, argNameRef, err := mod.PutLiteralBytes(argumentValue)
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
