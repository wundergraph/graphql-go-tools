/*
Package astnormalization helps to transform parsed GraphQL AST's into a easier to use structure.

# Example

This example shows how the normalization package helps "simplify" a GraphQL AST.

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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// NormalizeOperation creates a default Normalizer and applies all rules to a given AST
// In case you're using OperationNormalizer in a hot path, you shouldn't be using this function.
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
	ignoreSkipInclude                     bool
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

func WithIgnoreSkipInclude() Option {
	return func(options *options) {
		options.ignoreSkipInclude = true
	}
}

func (o *OperationNormalizer) setupOperationWalkers() {
	o.operationWalkers = make([]walkerStage, 0, 9)

	// NOTE: normalization rules for variables rely on the fact that
	// we will visit only a single operation, so it is important to remove non-matching operations
	if o.options.removeNotMatchingOperationDefinitions {
		removeNotMatchingOperationDefinitionsWalker := astvisitor.NewWalkerWithID(2, "RemoveNotMatchingOperationDefinitions")
		// This rule does not walk deep into ast, so a separate walk is not expensive,
		// but we could not mix this walk with other rules, because they need to go deep
		o.removeOperationDefinitionsVisitor = removeOperationDefinitions(&removeNotMatchingOperationDefinitionsWalker)

		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "removeNotMatchingOperationDefinitions",
			walker: &removeNotMatchingOperationDefinitionsWalker,
		})
	}

	directivesIncludeSkip := astvisitor.NewWalkerWithID(8, "DirectivesIncludeSkip")
	preventFragmentCycles(&directivesIncludeSkip)
	directiveIncludeSkipKeepNodes(&directivesIncludeSkip, o.options.ignoreSkipInclude)

	cleanup := astvisitor.NewWalkerWithID(8, "Cleanup")
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
		fragmentInline := astvisitor.NewWalkerWithID(8, "FragmentSpreadInline")
		fragmentSpreadInline(&fragmentInline)
		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "fragmentInline",
			walker: &fragmentInline,
		})
	}

	if o.options.extractVariables {
		extractVariablesWalker := astvisitor.NewWalkerWithID(8, "ExtractVariables")
		extractVariables(&extractVariablesWalker)
		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "extractVariables",
			walker: &extractVariablesWalker,
		})
	}

	other := astvisitor.NewWalkerWithID(8, "Other")
	removeSelfAliasing(&other)
	inlineSelectionsFromInlineFragments(&other)
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "removeSelfAliasing, inlineSelectionsFromInlineFragments",
		walker: &other,
	})

	mergeInlineFragments := astvisitor.NewWalkerWithID(8, "MergeInlineFragmentSelections")
	mergeInlineFragmentSelections(&mergeInlineFragments)
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "mergeInlineFragmentSelections",
		walker: &mergeInlineFragments,
	})

	if o.options.removeFragmentDefinitions {
		removeFragments := astvisitor.NewWalkerWithID(8, "RemoveFragmentDefinitions")
		removeFragmentDefinitions(&removeFragments)

		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:   "removeFragmentDefinitions",
			walker: &removeFragments,
		})
	}

	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "deduplicateFields, deleteUnusedVariables",
		walker: &cleanup,
	})

	if o.options.extractVariables {
		variablesProcessing := astvisitor.NewWalkerWithID(8, "VariablesProcessing")
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
		// printed, _ := astprinter.PrintStringIndent(operation, "  ")
		// fmt.Println("\n\nNormalizeOperation stage:", o.operationWalkers[i].name)
		// fmt.Println(printed)
		// fmt.Println("variables:", string(operation.Input.Variables))
	}
}

type VariablesNormalizer struct {
	firstDetectUnused          *astvisitor.Walker
	secondExtract              *astvisitor.Walker
	thirdDeleteUnused          *astvisitor.Walker
	fourthCoerce               *astvisitor.Walker
	variablesExtractionVisitor *variablesExtractionVisitor
}

func NewVariablesNormalizer() *VariablesNormalizer {
	// delete unused modifying variables refs,
	// so it is safer to run it sequentially with the extraction
	thirdDeleteUnused := astvisitor.NewWalkerWithID(8, "DeleteUnusedVariables")
	del := deleteUnusedVariables(&thirdDeleteUnused)

	// register variable usage detection on the first stage
	// and pass usage information to the deletion visitor
	// so it keeps variables that are defined but not used at all
	// ensuring that validation can still catch them
	firstDetectUnused := astvisitor.NewWalkerWithID(8, "DetectVariableUsage")
	detectVariableUsage(&firstDetectUnused, del)

	secondExtract := astvisitor.NewWalkerWithID(8, "ExtractVariables")
	variablesExtractionVisitor := extractVariables(&secondExtract)
	extractVariablesDefaultValue(&secondExtract)

	fourthCoerce := astvisitor.NewWalkerWithID(0, "VariablesCoercion")
	inputCoercionForList(&fourthCoerce)

	return &VariablesNormalizer{
		firstDetectUnused:          &firstDetectUnused,
		secondExtract:              &secondExtract,
		thirdDeleteUnused:          &thirdDeleteUnused,
		fourthCoerce:               &fourthCoerce,
		variablesExtractionVisitor: variablesExtractionVisitor,
	}
}

func (v *VariablesNormalizer) NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) []uploads.UploadPathMapping {
	v.firstDetectUnused.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil
	}
	v.secondExtract.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil
	}
	v.thirdDeleteUnused.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil
	}
	v.fourthCoerce.Walk(operation, definition, report)

	return v.variablesExtractionVisitor.uploadsPath
}

type fragmentCycleVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	currentFragmentRef    int           // current fragment ref
	spreadsInFragments    map[int][]int // fragment ref -> spread refs
}

func (f *fragmentCycleVisitor) LeaveDocument(operation, _ *ast.Document) {
	report := f.Walker.Report
	if report == nil {
		return
	}

	visited := make(map[string]bool)
	stack := make(map[string]bool)

	for fragmentIdx := range f.spreadsInFragments {
		f.detectFragmentCycle(fragmentIdx, []int{fragmentIdx}, visited, stack, operation)
	}
}

func (f *fragmentCycleVisitor) detectFragmentCycle(fragmentIdx int, path []int, visited, stack map[string]bool, operation *ast.Document) bool {
	fragName := string(operation.FragmentDefinitionNameBytes(fragmentIdx))
	if stack[fragName] {
		// Cycle detected, report using the spread that closes the cycle
		cycleStart := 0
		for i, idx := range path {
			if string(operation.FragmentDefinitionNameBytes(idx)) == fragName {
				cycleStart = i
				break
			}
		}
		cyclePath := path[cycleStart:]
		if len(cyclePath) > 0 {
			// The spread that closes the cycle is the first spread in the cycle
			cycleFragIdx := cyclePath[0]
			spreadName := operation.FragmentDefinitionNameBytes(cycleFragIdx)
			f.Walker.Report.AddExternalError(operationreport.ErrFragmentSpreadFormsCycle(spreadName))
		}
		return true
	}
	if visited[fragName] {
		return false
	}
	visited[fragName] = true
	stack[fragName] = true
	for _, spreadRef := range f.spreadsInFragments[fragmentIdx] {
		// Find the fragment definition index for this spread name
		fragName := operation.FragmentSpreadNameBytes(spreadRef)
		fragRef, exists := operation.FragmentDefinitionRef(fragName)
		if exists && f.detectFragmentCycle(fragRef, append(path, fragRef), visited, stack, operation) {
			return true
		}
	}
	stack[fragName] = false
	return false
}

func (f *fragmentCycleVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
	f.currentFragmentRef = -1
	f.spreadsInFragments = make(map[int][]int)
}

func (f *fragmentCycleVisitor) LeaveFragmentDefinition(ref int) {
	f.currentFragmentRef = -1
}

func (f *fragmentCycleVisitor) EnterFragmentDefinition(ref int) {
	f.currentFragmentRef = ref
}

func (f *fragmentCycleVisitor) EnterFragmentSpread(ref int) {
	if f.currentFragmentRef == -1 {
		return
	}
	if _, exists := f.spreadsInFragments[f.currentFragmentRef]; !exists {
		f.spreadsInFragments[f.currentFragmentRef] = []int{ref}
		return
	}
	f.spreadsInFragments[f.currentFragmentRef] = append(f.spreadsInFragments[f.currentFragmentRef], ref)
}

func preventFragmentCycles(walker *astvisitor.Walker) *fragmentCycleVisitor {
	visitor := &fragmentCycleVisitor{
		Walker:     walker,
		operation:  nil,
		definition: nil,
	}
	walker.RegisterDocumentVisitor(visitor)
	walker.RegisterEnterFragmentSpreadVisitor(visitor)
	walker.RegisterFragmentDefinitionVisitor(visitor)
	return visitor
}
