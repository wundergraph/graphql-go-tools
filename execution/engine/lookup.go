package engine

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
)

type TypeFieldLookupKey string

func CreateTypeFieldLookupKey(typeName string, fieldName string) TypeFieldLookupKey {
	return TypeFieldLookupKey(fmt.Sprintf("%s.%s", typeName, fieldName))
}

func CreateTypeFieldArgumentsLookupMap(typeFieldArgs []graphql.TypeFieldArguments) map[TypeFieldLookupKey]graphql.TypeFieldArguments {
	if len(typeFieldArgs) == 0 {
		return nil
	}

	lookupMap := make(map[TypeFieldLookupKey]graphql.TypeFieldArguments)
	for _, currentTypeFieldArgs := range typeFieldArgs {
		lookupMap[CreateTypeFieldLookupKey(currentTypeFieldArgs.TypeName, currentTypeFieldArgs.FieldName)] = currentTypeFieldArgs
	}

	return lookupMap
}
