package parser

import (
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

	/*	if objectTypeDefinition.Name == "Query" {
		introspectionFields := document.FieldsDefinition{
			{
				Name: "__schema",
				Type: document.NamedType{
					Name:    "__Schema",
					NonNull: true,
				},
			},
			{
				Name: "__type",
				Type: document.NamedType{
					Name:    "__Type",
					NonNull: false,
				},
				ArgumentsDefinition: []document.InputValueDefinition{
					{
						Name: "name",
						Type: document.NamedType{
							Name:    "String",
							NonNull: true,
						},
					},
				},
			},
		}

		objectTypeDefinition.FieldsDefinition = append(introspectionFields, objectTypeDefinition.FieldsDefinition...)
	}*/

	return
}
