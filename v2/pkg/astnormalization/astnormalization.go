/*
Package astnormalization helps to transform parsed GraphQL AST's into a easier to use structure.

# Example

This examples shows how the normalization package helps "simplifying" a GraphQL AST.

Input:

	subscription sub {
		... multipleSubscriptions
		... on Subscription {
			newMessage {
				body
				sender
			}
		}
	}
	fragment newMessageFields on Message {
		body: body
		sender
		... on Body {
			body
		}
	}
	fragment multipleSubscriptions on Subscription {
		newMessage {
			body
			sender
		}
		newMessage {
			... newMessageFields
		}
		newMessage {
			body
			body
			sender
		}
		... on Subscription {
			newMessage {
				body
				sender
			}
		}
		disallowedSecondRootField
	}

Output:

	subscription sub {
		newMessage {
			body
			sender
		}
		disallowedSecondRootField
	}
	fragment newMessageFields on Message {
		body
		sender
	}
	fragment multipleSubscriptions on Subscription {
		newMessage {
			body
			sender
		}
		disallowedSecondRootField
	}
*/
package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// NormalizeOperation creates a default Normalizer and applies all rules to a given AST
// In case you're using OperationNormalizer in a hot path you shouldn't be using this function.
// Create a new OperationNormalizer using NewNormalizer() instead and re-use it.
func NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) {
	normalizer := NewNormalizer(false, false)
	normalizer.NormalizeOperation(operation, definition, report)

	// TODO: change to use NewWithOpts
	// TODO: set a default name for the operation
}

func NormalizeNamedOperation(operation, definition *ast.Document, operationName []byte, report *operationreport.Report) {
	normalizer := NewWithOpts(
		WithRemoveNotMatchingOperationDefinitions(),
		WithExtractVariables(),
		WithRemoveFragmentDefinitions(),
		WithInlineFragmentSpreads(),
		WithRemoveUnusedVariables(),
	)
	normalizer.NormalizeNamedOperation(operation, definition, operationName, report)
}

type walkerStage struct {
	name   string
	walker *astvisitor.Walker
}

// OperationNormalizer walks a given AST and applies all registered rules
type OperationNormalizer struct {
	operationWalkers []walkerStage

	removeOperationDefinitionsVisitor *removeOperationDefinitionsVisitor

	options              options
	definitionNormalizer *DefinitionNormalizer
}

// NewNormalizer creates a new OperationNormalizer and sets up all default rules
func NewNormalizer(removeFragmentDefinitions, extractVariables bool) *OperationNormalizer {
	normalizer := &OperationNormalizer{
		options: options{
			removeFragmentDefinitions: removeFragmentDefinitions,
			inlineFragmentSpreads:     true,
			extractVariables:          extractVariables,
		},
	}
	normalizer.setupOperationWalkers()
	return normalizer
}

// NewWithOpts creates a new OperationNormalizer with Options
func NewWithOpts(opts ...Option) *OperationNormalizer {
	var options options
	for _, opt := range opts {
		opt(&options)
	}
	normalizer := &OperationNormalizer{
		options: options,
	}
	normalizer.setupOperationWalkers()

	if options.normalizeDefinition {
		normalizer.definitionNormalizer = NewDefinitionNormalizer()
	}

	return normalizer
}

type options struct {
	removeFragmentDefinitions             bool
	inlineFragmentSpreads                 bool
	extractVariables                      bool
	removeUnusedVariables                 bool
	removeNotMatchingOperationDefinitions bool
	normalizeDefinition                   bool
}

type Option func(options *options)

func WithExtractVariables() Option {
	return func(options *options) {
		options.extractVariables = true
	}
}

func WithRemoveFragmentDefinitions() Option {
	return func(options *options) {
		options.removeFragmentDefinitions = true
	}
}

func WithInlineFragmentSpreads() Option {
	return func(options *options) {
		options.inlineFragmentSpreads = true
	}
}

func WithRemoveUnusedVariables() Option {
	return func(options *options) {
		options.removeUnusedVariables = true
	}
}

func WithRemoveNotMatchingOperationDefinitions() Option {
	return func(options *options) {
		options.removeNotMatchingOperationDefinitions = true
	}
}

func WithNormalizeDefinition() Option {
	return func(options *options) {
		options.normalizeDefinition = true
	}
}

func (o *OperationNormalizer) setupOperationWalkers() {
	o.operationWalkers = make([]walkerStage, 0, 9)

	// NOTE: normalization rules for variables relies on the fact that
	// we will visit only single operation, so it is important to remove non-matching operations
	if o.options.removeNotMatchingOperationDefinitions {
		removeNotMatchingOperationDefinitionsWalker := astvisitor.NewWalker(2)
		// this rule do not walk deep into ast, so separate walk not expensive,
		// but we could not mix this walk with other rules, because they need to go deep
		o.removeOperationDefinitionsVisitor = removeOperationDefinitions(&removeNotMatchingOperationDefinitionsWalker)

		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "removeNotMatchingOperationDefinitions",
			walker: &removeNotMatchingOperationDefinitionsWalker,
		})
	}

	directivesIncludeSkip := astvisitor.NewWalker(8)
	directiveIncludeSkip(&directivesIncludeSkip)

	cleanup := astvisitor.NewWalker(8)
	mergeFieldSelections(&cleanup)
	deduplicateFields(&cleanup)
	if o.options.removeUnusedVariables {
		del := deleteUnusedVariables(&cleanup)
		// register variable usage detection on the first stage
		// and pass usage information to the deletion visitor
		// so it keeps variables that are defined but not used at all
		// ensuring that validation can still catch them
		detectVariableUsage(&directivesIncludeSkip, del)
	}

	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "directivesIncludeSkip, removeOperationDefinitions",
		walker: &directivesIncludeSkip,
	})

	if o.options.inlineFragmentSpreads {
		fragmentInline := astvisitor.NewWalker(8)
		fragmentSpreadInline(&fragmentInline)
		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "fragmentInline",
			walker: &fragmentInline,
		})
	}

	if o.options.extractVariables {
		extractVariablesWalker := astvisitor.NewWalker(8)
		extractVariables(&extractVariablesWalker)
		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "extractVariables",
			walker: &extractVariablesWalker,
		})
	}

	other := astvisitor.NewWalker(8)
	removeSelfAliasing(&other)
	inlineSelectionsFromInlineFragments(&other)
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "removeSelfAliasing, inlineSelectionsFromInlineFragments",
		walker: &other,
	})

	mergeInlineFragments := astvisitor.NewWalker(8)
	mergeInlineFragmentSelections(&mergeInlineFragments)
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "mergeInlineFragmentSelections",
		walker: &mergeInlineFragments,
	})

	if o.options.removeFragmentDefinitions {
		removeFragments := astvisitor.NewWalker(8)
		removeFragmentDefinitions(&removeFragments)

		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "removeFragmentDefinitions",
			walker: &removeFragments,
		})
	}

	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "mergeFieldSelections, deduplicateFields, deleteUnusedVariables",
		walker: &cleanup,
	})

	if o.options.extractVariables {
		variablesProcessing := astvisitor.NewWalker(8)
		inputCoercionForList(&variablesProcessing)
		extractVariablesDefaultValue(&variablesProcessing)
		injectInputFieldDefaults(&variablesProcessing)

		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "variablesProcessing",
			walker: &variablesProcessing,
		})
	}
}

func (o *OperationNormalizer) prepareDefinition(definition *ast.Document, report *operationreport.Report) {
	if o.definitionNormalizer != nil {
		o.definitionNormalizer.NormalizeDefinition(definition, report)
	}
}

// NormalizeOperation applies all registered rules to the AST
func (o *OperationNormalizer) NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) {
	if o.options.normalizeDefinition {
		o.prepareDefinition(definition, report)
		if report.HasErrors() {
			return
		}
	}

	for i := range o.operationWalkers {
		o.operationWalkers[i].walker.Walk(operation, definition, report)
		if report.HasErrors() {
			return
		}
	}
}

// NormalizeNamedOperation applies all registered rules to one specific named operation in the AST
func (o *OperationNormalizer) NormalizeNamedOperation(operation, definition *ast.Document, operationName []byte, report *operationreport.Report) {
	if o.options.normalizeDefinition {
		o.prepareDefinition(definition, report)
		if report.HasErrors() {
			return
		}
	}

	if o.removeOperationDefinitionsVisitor != nil {
		o.removeOperationDefinitionsVisitor.operationName = operationName
	}

	for i := range o.operationWalkers {
		o.operationWalkers[i].walker.Walk(operation, definition, report)
		if report.HasErrors() {
			return
		}

		// NOTE: debug code - do not remove
		// printed, _ := astprinter.PrintStringIndent(operation, definition, "  ")
		// fmt.Println("\n\nNormalizeOperation stage:", o.operationWalkers[i].name)
		// fmt.Println(printed)
		// fmt.Println("variables:", string(operation.Input.Variables))
	}
}

type VariablesNormalizer struct {
	firstDetectUnused *astvisitor.Walker
	secondExtract     *astvisitor.Walker
	thirdDeleteUnused *astvisitor.Walker
	fourthCoerce      *astvisitor.Walker
}

func NewVariablesNormalizer() *VariablesNormalizer {
	// delete unused modifying variables refs,
	// so it is safer to run it sequentially with the extraction
	thirdDeleteUnused := astvisitor.NewWalker(8)
	del := deleteUnusedVariables(&thirdDeleteUnused)

	// register variable usage detection on the first stage
	// and pass usage information to the deletion visitor
	// so it keeps variables that are defined but not used at all
	// ensuring that validation can still catch them
	firstDetectUnused := astvisitor.NewWalker(8)
	detectVariableUsage(&firstDetectUnused, del)

	secondExtract := astvisitor.NewWalker(8)
	extractVariables(&secondExtract)
	extractVariablesDefaultValue(&secondExtract)

	fourthCoerce := astvisitor.NewWalker(0)
	inputCoercionForList(&fourthCoerce)

	return &VariablesNormalizer{
		firstDetectUnused: &firstDetectUnused,
		secondExtract:     &secondExtract,
		thirdDeleteUnused: &thirdDeleteUnused,
		fourthCoerce:      &fourthCoerce,
	}
}

func (v *VariablesNormalizer) NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) {
	v.firstDetectUnused.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}
	v.secondExtract.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}
	v.thirdDeleteUnused.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}
	v.fourthCoerce.Walk(operation, definition, report)
}
