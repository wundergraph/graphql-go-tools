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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
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
		WithExtractVariables(),
		WithRemoveFragmentDefinitions(),
		WithInlineFragmentSpreads(),
		WithRemoveNotMatchingOperationDefinitions(),
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

	variablesExtraction               *variablesExtractionVisitor
	variablesDefaultValuesExtraction  *variablesDefaultValueExtractionVisitor
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
	o.operationWalkers = make([]walkerStage, 0, 6)

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

	if o.options.removeNotMatchingOperationDefinitions {
		o.removeOperationDefinitionsVisitor = removeOperationDefinitions(&directivesIncludeSkip)
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
		o.variablesExtraction = extractVariables(&extractVariablesWalker)
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
		o.variablesDefaultValuesExtraction = extractVariablesDefaultValue(&variablesProcessing)
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

	if o.variablesExtraction != nil {
		o.variablesExtraction.operationName = operationName
	}
	if o.variablesDefaultValuesExtraction != nil {
		o.variablesDefaultValuesExtraction.operationName = operationName
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
	pre    *astvisitor.Walker
	post   *astvisitor.Walker
	coerce *astvisitor.Walker

	detect                  *variableUsageDetector
	del                     *deleteUnusedVariablesVisitor
	extractVariables        *variablesExtractionVisitor
	extractDefaultVariables *variablesDefaultValueExtractionVisitor
}

func NewVariablesNormalizer() *VariablesNormalizer {
	pre := astvisitor.NewWalker(8)
	post := astvisitor.NewWalker(8)
	coerce := astvisitor.NewWalker(0)
	ex := extractVariables(&post)
	def := extractVariablesDefaultValue(&post)
	del := deleteUnusedVariables(&post)
	det := detectVariableUsage(&pre, del)
	inputCoercionForList(&coerce)
	return &VariablesNormalizer{
		pre:                     &pre,
		post:                    &post,
		coerce:                  &coerce,
		detect:                  det,
		del:                     del,
		extractVariables:        ex,
		extractDefaultVariables: def,
	}
}

func (v *VariablesNormalizer) NormalizeNamedOperation(operation, definition *ast.Document, operationName string, report *operationreport.Report) {
	operationNameBytes := unsafebytes.StringToBytes(operationName)
	v.extractVariables.operationName = operationNameBytes
	v.extractDefaultVariables.operationName = operationNameBytes
	v.detect.operationName = operationNameBytes
	v.del.operationName = operationNameBytes
	v.pre.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}
	v.post.Walk(operation, definition, report)
	v.coerce.Walk(operation, definition, report)
}
