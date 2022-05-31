package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type primaryKeySet map[string]bool

type entitySet map[string]primaryKeySet

type entityValidator struct {
	document  *ast.Document
	entitySet entitySet
}

func (e *entityValidator) setDocument(document *ast.Document) {
	if e.document == nil {
		e.document = document
	}
}

func (e *entityValidator) getPrimaryKeys(name string, directiveRefs []int, isExtension bool) (primaryKeySet, *operationreport.ExternalError) {
	primaryKeys := make(primaryKeySet)
	for _, directiveRef := range directiveRefs {
		if e.document.DirectiveNameString(directiveRef) != keyDirectiveName {
			continue
		}
		directive := e.document.Directives[directiveRef]
		if len(directive.Arguments.Refs) != 1 {
			err := operationreport.ErrKeyDirectiveMustHaveSingleFieldsArgument(name)
			return nil, &err
		}
		argumentRef := directive.Arguments.Refs[0]
		if e.document.ArgumentNameString(argumentRef) != keyDirectiveArgument {
			err := operationreport.ErrKeyDirectiveMustHaveSingleFieldsArgument(name)
			return nil, &err
		}
		primaryKey := e.document.StringValueContentString(e.document.Arguments[argumentRef].Value.Ref)
		if primaryKey == "" {
			err := operationreport.ErrPrimaryKeyMustNotBeEmpty(name)
			return nil, &err
		}
		if isExtension {
			if _, exists := e.entitySet[name][primaryKey]; !exists {
				err := operationreport.ErrPrimaryKeyReferencesMustExistOnEntity(primaryKey, name)
				return nil, &err
			}
		}
		primaryKeys[primaryKey] = false
	}
	return primaryKeys, nil
}

func (e *entityValidator) validatePrimaryKeyReferences(name string, fieldRefs []int) *operationreport.ExternalError {
	primaryKeys := e.entitySet[name]
	fieldReferences := len(primaryKeys)
	if fieldReferences < 1 {
		return nil
	}
	for _, fieldRef := range fieldRefs {
		fieldName := e.document.FieldDefinitionNameString(fieldRef)
		isResolved, isPrimaryKey := primaryKeys[fieldName]
		if !isPrimaryKey {
			continue
		}
		if !isResolved {
			primaryKeys[fieldName] = true
			fieldReferences -= 1
		}
		if fieldReferences == 0 {
			return nil
		}
	}
	for primaryKey, isResolved := range primaryKeys {
		if !isResolved {
			err := operationreport.ErrPrimaryKeyReferencesMustExistOnEntity(primaryKey, name)
			return &err
		}
	}
	return nil
}

func (e *entityValidator) isEntityExtension(directiveRefs []int) bool {
	for _, directiveRef := range directiveRefs {
		if e.document.DirectiveNameString(directiveRef) == keyDirectiveName {
			return true
		}
	}
	return false
}

func (e *entityValidator) validateExternalPrimaryKeys(name string, primaryKeys primaryKeySet, fieldRefs []int) *operationreport.ExternalError {
	fieldReferences := len(primaryKeys)
	if fieldReferences < 1 {
		err := operationreport.ErrEntityExtensionMustHaveKeyDirective(name)
		return &err
	}
	for _, fieldRef := range fieldRefs {
		fieldName := e.document.FieldDefinitionNameString(fieldRef)
		isExternalDirectiveResolved, isPrimaryKey := primaryKeys[fieldName]
		if !isPrimaryKey {
			continue
		}
		field := e.document.FieldDefinitions[fieldRef]
		hasExternalDirective := false
		for _, directiveRef := range field.Directives.Refs {
			if e.document.DirectiveNameString(directiveRef) != externalDirective {
				continue
			}
			hasExternalDirective = true
			if !isExternalDirectiveResolved {
				primaryKeys[fieldName] = true
				fieldReferences -= 1
			}
			if fieldReferences == 0 {
				return nil
			}
			break
		}
		if !hasExternalDirective {
			err := operationreport.ErrEntityExtensionPrimaryKeyFieldReferenceMustHaveExternalDirective(name)
			return &err
		}
	}
	err := operationreport.ErrEntityExtensionPrimaryKeyMustExistAsField(name)
	return &err
}
