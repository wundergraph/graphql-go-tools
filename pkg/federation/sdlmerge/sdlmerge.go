package sdlmerge

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

const (
	rootOperationTypeDefinitions = `
		type Query {}
		type Mutation {}
		type Subscription {}
	`

	parseDocumentError = "parse graphql document string: %s"

	keyDirectiveName = "key"

	keyDirectiveArgument = "fields"

	externalDirective = "external"
)

type Visitor interface {
	Register(walker *astvisitor.Walker)
}

func MergeAST(ast *ast.Document) error {
	normalizer := normalizer{}
	normalizer.setupWalkers()

	return normalizer.normalize(ast)
}

func MergeSDLs(SDLs ...string) (string, error) {
	rawDocs := make([]string, 0, len(SDLs)+1)
	rawDocs = append(rawDocs, rootOperationTypeDefinitions)
	rawDocs = append(rawDocs, SDLs...)
	if validationError := validateSubgraphs(rawDocs[1:]); validationError != nil {
		return "", validationError
	}
	if normalizationError := normalizeSubgraphs(rawDocs[1:]); normalizationError != nil {
		return "", normalizationError
	}

	doc, report := astparser.ParseGraphqlDocumentString(strings.Join(rawDocs, "\n"))
	if report.HasErrors() {
		return "", fmt.Errorf("parse graphql document string: %s", report.Error())
	}

	astnormalization.NormalizeSubgraphSDL(&doc, &report)
	if report.HasErrors() {
		return "", fmt.Errorf("merge ast: %s", report.Error())
	}

	if err := MergeAST(&doc); err != nil {
		return "", fmt.Errorf("merge ast: %s", err.Error())
	}

	out, err := astprinter.PrintString(&doc, nil)
	if err != nil {
		return "", fmt.Errorf("stringify schema: %s", err.Error())
	}

	return out, nil
}

func validateSubgraphs(subgraphs []string) error {
	validator := astvalidation.NewDefinitionValidator(
		astvalidation.PopulatedTypeBodies(), astvalidation.KnownTypeNames(),
	)
	for _, subgraph := range subgraphs {
		doc, report := astparser.ParseGraphqlDocumentString(subgraph)
		if err := asttransform.MergeDefinitionWithBaseSchema(&doc); err != nil {
			return err
		}
		if report.HasErrors() {
			return fmt.Errorf(parseDocumentError, report.Error())
		}
		validator.Validate(&doc, &report)
		if report.HasErrors() {
			return fmt.Errorf("validate schema: %s", report.Error())
		}
	}
	return nil
}

func normalizeSubgraphs(subgraphs []string) error {
	subgraphNormalizer := astnormalization.NewSubgraphDefinitionNormalizer()
	for i, subgraph := range subgraphs {
		doc, report := astparser.ParseGraphqlDocumentString(subgraph)
		if report.HasErrors() {
			return fmt.Errorf(parseDocumentError, report.Error())
		}
		subgraphNormalizer.NormalizeDefinition(&doc, &report)
		if report.HasErrors() {
			return fmt.Errorf("normalize schema: %s", report.Error())
		}
		out, err := astprinter.PrintString(&doc, nil)
		if err != nil {
			return fmt.Errorf("stringify schema: %s", err.Error())
		}
		subgraphs[i] = out
	}
	return nil
}

type normalizer struct {
	walkers         []*astvisitor.Walker
	entityValidator *entityValidator
}

func (m *normalizer) setupWalkers() {
	m.entityValidator = &entityValidator{entitySet: make(entitySet)}
	visitorGroups := [][]Visitor{
		{
			newCollectValidEntitiesVisitor(m),
		},
		{
			newExtendEnumTypeDefinition(),
			newExtendInputObjectTypeDefinition(),
			newExtendInterfaceTypeDefinition(m),
			newExtendScalarTypeDefinition(),
			newExtendUnionTypeDefinition(),
			newExtendObjectTypeDefinition(m),
			newRemoveEmptyObjectTypeDefinition(),
			newRemoveMergedTypeExtensions(),
		},
		// visitors for cleaning up federated duplicated fields and directives
		{
			newRemoveFieldDefinitions("external"),
			newRemoveDuplicateFieldedSharedTypesVisitor(),
			newRemoveDuplicateFieldlessSharedTypesVisitor(),
			newRemoveInterfaceDefinitionDirective("key"),
			newRemoveObjectTypeDefinitionDirective("key"),
			newRemoveFieldDefinitionDirective("provides", "requires"),
		},
	}

	for _, visitorGroup := range visitorGroups {
		walker := astvisitor.NewWalker(48)
		for _, visitor := range visitorGroup {
			visitor.Register(&walker)
			m.walkers = append(m.walkers, &walker)
		}
	}
}

func (m *normalizer) normalize(operation *ast.Document) error {
	report := operationreport.Report{}

	for _, walker := range m.walkers {
		walker.Walk(operation, nil, &report)
		if report.HasErrors() {
			return fmt.Errorf("walk: %s", report.Error())
		}
	}

	return nil
}
