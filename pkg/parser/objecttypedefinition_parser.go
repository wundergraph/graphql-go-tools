package parser

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseObjectTypeDefinition() (objectTypeDefinition document.ObjectTypeDefinition, err error) {

	objectTypeName, err := p.readExpect(keyword.IDENT, "parseObjectTypeDefinition")
	if err != nil {
		return
	}

	objectTypeDefinition.Name = objectTypeName.Literal

	objectTypeDefinition.ImplementsInterfaces, err = p.parseImplementsInterfaces()
	if err != nil {
		return
	}

	objectTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	objectTypeDefinition.FieldsDefinition, err = p.parseFieldsDefinition()
	if err != nil {
		return objectTypeDefinition, err
	}

	if bytes.Equal(objectTypeDefinition.Name, []byte("Query")) {
		introspectionFields := document.FieldsDefinition{
			{
				Name: []byte("__schema"),
				Type: document.NamedType{
					Name:    []byte("__Schema"),
					NonNull: true,
				},
			},
			{
				Name: []byte("__type"),
				Type: document.NamedType{
					Name:    []byte("__Type"),
					NonNull: false,
				},
				ArgumentsDefinition: []document.InputValueDefinition{
					{
						Name: []byte("name"),
						Type: document.NamedType{
							Name:    []byte("String"),
							NonNull: true,
						},
					},
				},
			},
		}

		objectTypeDefinition.FieldsDefinition = append(introspectionFields, objectTypeDefinition.FieldsDefinition...)
	}

	return
}
