package sdlmerge

import (
	"fmt"
	"github.com/wundergraph/graphql-go-tools/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"strings"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

const (
	rootOperationTypeDefinitions = `
		type Query {}
		type Mutation {}
		type Subscription {}
	`

	parseDocumentError = "parse graphql document string: %w"
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
		return "", fmt.Errorf("parse graphql document string: %w", report)
	}

	astnormalization.NormalizeSubgraphSDL(&doc, &report)
	if report.HasErrors() {
		return "", fmt.Errorf("merge ast: %w", report)
	}

	if err := MergeAST(&doc); err != nil {
		return "", fmt.Errorf("merge ast: %w", err)
	}

	out, err := astprinter.PrintString(&doc, nil)
	if err != nil {
		return "", fmt.Errorf("stringify schema: %w", err)
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
			return fmt.Errorf(parseDocumentError, report)
		}
		validator.Validate(&doc, &report)
		if report.HasErrors() {
			return fmt.Errorf("validate schema: %w", report)
		}
	}
	return nil
}

func normalizeSubgraphs(subgraphs []string) error {
	subgraphNormalizer := astnormalization.NewSubgraphDefinitionNormalizer()
	for i, subgraph := range subgraphs {
		doc, report := astparser.ParseGraphqlDocumentString(subgraph)
		if report.HasErrors() {
			return fmt.Errorf(parseDocumentError, report)
		}
		subgraphNormalizer.NormalizeDefinition(&doc, &report)
		if report.HasErrors() {
			return fmt.Errorf("normalize schema: %w", report)
		}
		out, err := astprinter.PrintString(&doc, nil)
		if err != nil {
			return fmt.Errorf("stringify schema: %w", err)
		}
		subgraphs[i] = out
	}
	return nil
}

type normalizer struct {
	walkers []*astvisitor.Walker
}

type entitySet map[string]struct{}

func (m *normalizer) setupWalkers() {
	collectedEntities := make(entitySet)
	visitorGroups := [][]Visitor{
		{
			newCollectEntitiesVisitor(collectedEntities),
		},
		{
			newExtendEnumTypeDefinition(),
			newExtendInputObjectTypeDefinition(),
			newExtendInterfaceTypeDefinition(collectedEntities),
			newExtendScalarTypeDefinition(),
			newExtendUnionTypeDefinition(),
			newExtendObjectTypeDefinition(collectedEntities),
			newRemoveEmptyObjectTypeDefinition(),
			newRemoveMergedTypeExtensions(),
		},
		// visitors for cleaning up federated duplicated fields and directives
		{
			newRemoveFieldDefinitions("external"),
			newRemoveDuplicateFieldedSharedTypesVisitor(),
			newRemoveDuplicateFieldlessSharedTypesVisitor(),
			newMergeDuplicatedFieldsVisitor(),
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
			return fmt.Errorf("walk: %w", report)
		}
	}

	return nil
}

func (e entitySet) isExtensionForEntity(nameBytes []byte, directiveRefs []int, document *ast.Document) (bool, *operationreport.ExternalError) {
	name := string(nameBytes)
	hasDirectives := len(directiveRefs) > 0
	if _, exists := e[name]; !exists {
		if !hasDirectives || !isEntityExtension(directiveRefs, document) {
			return false, nil
		}
		err := operationreport.ErrExtensionWithKeyDirectiveMustExtendEntity(name)
		return false, &err
	}
	if !hasDirectives {
		err := operationreport.ErrEntityExtensionMustHaveKeyDirective(name)
		return false, &err
	}
	if isEntityExtension(directiveRefs, document) {
		return true, nil
	}
	err := operationreport.ErrEntityExtensionMustHaveKeyDirective(name)
	return false, &err
}

func isEntityExtension(directiveRefs []int, document *ast.Document) bool {
	for _, directiveRef := range directiveRefs {
		if document.DirectiveNameString(directiveRef) == plan.FederationKeyDirectiveName {
			return true
		}
	}
	return false
}

func multipleExtensionError(isEntity bool, nameBytes []byte) *operationreport.ExternalError {
	if isEntity {
		err := operationreport.ErrEntitiesMustNotBeDuplicated(string(nameBytes))
		return &err
	}
	err := operationreport.ErrSharedTypesMustNotBeExtended(string(nameBytes))
	return &err
}
