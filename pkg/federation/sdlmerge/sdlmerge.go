package sdlmerge

import (
	"fmt"
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

var (
	federationPredeclaredFieldDirectives      = []string{"external", "requires", "provides"}
	federationPredeclaredObjectTypeDirectives = []string{"key"}
)

const rootOperationTypeDefinitions = `
	type Query {}
	type Mutation {}
	type Subscription {}
`

type Visitor interface {
	Register(walker *astvisitor.Walker)
}

func MergeAST(ast *ast.Document) error {
	normalizer := normalizer{}
	normalizer.setupWalkers()

	return normalizer.normalize(ast)
}

func MergeSDLs(SDLs ...string) (string, error) {
	rawDocs := make([]string, 0, len(SDLs) + 1)
	rawDocs = append(rawDocs, rootOperationTypeDefinitions)
	rawDocs = append(rawDocs, SDLs...)

	doc, report := astparser.ParseGraphqlDocumentString(strings.Join(rawDocs, "\n"))
	if report.HasErrors() {
		return "", fmt.Errorf("parse graphql document string: %w", report)
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

type normalizer struct {
	walkers []*astvisitor.Walker
}

func (m *normalizer) setupWalkers() {
	visitorGroups := [][]Visitor{
		// visitors for extending objects and interfaces
		{
			newExtendInterfaceTypeDefinition(),
			newExtendObjectTypeDefinition(),
			newRemoveMergedTypeExtensions(),
			newRemoveEmptyObjectTypeDefinition(),
		},
		// visitors for clean up federated duplicated fields and directives
		{
			newRemoveFieldDefinitions(federationPredeclaredFieldDirectives...),
			newRemoveInterfaceDefinitionDirective(federationPredeclaredObjectTypeDirectives...),
			newRemoveObjectTypeDefinitionDirective(federationPredeclaredObjectTypeDirectives...),
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
