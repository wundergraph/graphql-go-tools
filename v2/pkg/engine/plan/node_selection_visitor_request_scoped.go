package plan

import (
	"slices"
	"sort"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type requestScopedGroupKey struct {
	l1Key  string
	dsHash DSHash
}

type requestScopedParticipant struct {
	fieldRef        int
	selectionSetRef int
	enclosingType   string
	fieldTypeName   string
	dsHash          DSHash
	path            string
}

type participantMissing struct {
	participant     requestScopedParticipant
	missingFragment string
}

type requestScopedSelectionUnion struct {
	variants         map[string]*requestScopedUnionVariant
	responseKeyIndex map[string]map[string]struct{}
}

type requestScopedUnionVariant struct {
	key                  string
	schemaFieldName      string
	argsPrinted          string
	directivesPrinted    string
	observedResponseKeys map[string]struct{}
	subSelection         *requestScopedSelectionUnion
}

type requestScopedSelectionSnapshot struct {
	fieldRefsByVariantKey map[string]int
	responseKeys          map[string]struct{}
}

func (c *nodeSelectionVisitor) propagateRequestScopedWidening() {
	groups := c.collectRequestScopedParticipants()
	for key, group := range groups {
		missing, ok := c.computeRequestScopedMissing(group)
		if !ok {
			continue
		}

		for _, item := range missing {
			if item.missingFragment == "" {
				continue
			}

			c.addFieldRequirementsToOperation(item.participant.selectionSetRef, fieldRequirements{
				dsHash:               key.dsHash,
				typeName:             item.participant.fieldTypeName,
				path:                 item.participant.path,
				selectionSet:         item.missingFragment,
				requestedByFieldRefs: nil,
			})
			if c.walker.Report != nil && c.walker.Report.HasErrors() {
				return
			}
		}
	}
}

func (c *nodeSelectionVisitor) collectRequestScopedParticipants() map[requestScopedGroupKey][]requestScopedParticipant {
	out := make(map[requestScopedGroupKey][]requestScopedParticipant)

	for _, rootNode := range c.operation.RootNodes {
		if rootNode.Kind != ast.NodeKindOperationDefinition {
			continue
		}

		operationDefinition := c.operation.OperationDefinitions[rootNode.Ref]
		operationName := c.operation.OperationDefinitionNameString(rootNode.Ref)
		if c.operationName != "" && c.operationName != operationName {
			continue
		}

		rootTypeNode, ok := c.rootOperationTypeNode(operationDefinition.OperationType)
		if !ok || !operationDefinition.HasSelections {
			continue
		}

		c.collectRequestScopedParticipantsInSelectionSet(operationDefinition.SelectionSet, rootTypeNode, operationDefinition.OperationType.Name(), out)
	}

	return out
}

func (c *nodeSelectionVisitor) collectRequestScopedParticipantsInSelectionSet(selectionSetRef int, enclosingTypeNode ast.Node, parentPath string, out map[requestScopedGroupKey][]requestScopedParticipant) {
	enclosingTypeName := enclosingTypeNode.NameString(c.definition)

	for _, selectionRef := range c.operation.SelectionSetFieldSelections(selectionSetRef) {
		fieldRef := c.operation.Selections[selectionRef].Ref
		fieldName := c.operation.FieldNameString(fieldRef)
		currentPath := parentPath + "." + c.operation.FieldAliasOrNameString(fieldRef)

		fieldDefinitionRef, exists := c.definition.NodeFieldDefinitionByName(enclosingTypeNode, c.operation.FieldNameBytes(fieldRef))
		if !exists {
			continue
		}

		fieldTypeName := c.definition.FieldDefinitionTypeNameString(fieldDefinitionRef)
		if fieldSelectionSetRef, ok := c.operation.FieldSelectionSet(fieldRef); ok {
			for _, ds := range c.dataSources {
				fedMeta := ds.FederationConfiguration()
				l1Keys := fedMeta.RequestScopedExportsForField(enclosingTypeName, fieldName)
				if len(l1Keys) == 0 {
					for _, io := range fedMeta.InterfaceObjects {
						if slices.Contains(io.ConcreteTypeNames, enclosingTypeName) {
							l1Keys = fedMeta.RequestScopedExportsForField(io.InterfaceTypeName, fieldName)
							if len(l1Keys) > 0 {
								break
							}
						}
					}
				}

				for _, l1Key := range l1Keys {
					key := requestScopedGroupKey{l1Key: l1Key, dsHash: ds.Hash()}
					out[key] = append(out[key], requestScopedParticipant{
						fieldRef:        fieldRef,
						selectionSetRef: fieldSelectionSetRef,
						enclosingType:   enclosingTypeName,
						fieldTypeName:   fieldTypeName,
						dsHash:          ds.Hash(),
						path:            currentPath,
					})
				}
			}

			fieldTypeNode, ok := c.definition.Index.FirstNodeByNameStr(fieldTypeName)
			if ok {
				c.collectRequestScopedParticipantsInSelectionSet(fieldSelectionSetRef, fieldTypeNode, currentPath, out)
			}
		}
	}
}

func (c *nodeSelectionVisitor) rootOperationTypeNode(operationType ast.OperationType) (ast.Node, bool) {
	switch operationType {
	case ast.OperationTypeQuery:
		return c.definition.NodeByName(c.definition.Index.QueryTypeName)
	case ast.OperationTypeMutation:
		return c.definition.NodeByName(c.definition.Index.MutationTypeName)
	case ast.OperationTypeSubscription:
		return c.definition.NodeByName(c.definition.Index.SubscriptionTypeName)
	default:
		return ast.InvalidNode, false
	}
}

func (c *nodeSelectionVisitor) computeRequestScopedMissing(group []requestScopedParticipant) ([]participantMissing, bool) {
	if len(group) < 2 {
		return nil, true
	}

	returnTypeName := group[0].fieldTypeName
	for _, participant := range group[1:] {
		if participant.fieldTypeName != returnTypeName {
			return nil, false
		}
	}

	ds, ok := c.dataSourceByHash(group[0].dsHash)
	if !ok {
		return nil, false
	}

	typeNode, ok := c.definition.Index.FirstNodeByNameStr(returnTypeName)
	if !ok {
		return nil, false
	}

	union := newRequestScopedSelectionUnion()
	for _, participant := range group {
		if !union.mergeSelectionSet(c.operation, c.definition, participant.selectionSetRef, typeNode, ds) {
			return nil, false
		}
		if !c.mergeRequestScopedRequiresSelectionSet(union, c.operation, participant.selectionSetRef, typeNode, ds) {
			return nil, false
		}
	}

	syntheticAliases := union.syntheticAliases()
	if len(syntheticAliases) > 0 {
		for _, participant := range group {
			if !union.recordExistingSelectionAliases(c.operation, c.definition, participant.selectionSetRef, typeNode, ds, syntheticAliases, c.requestScopedVisibleResponseKeys, c.requestScopedFetchAliases) {
				return nil, false
			}
		}
	}

	out := make([]participantMissing, 0, len(group))
	for _, participant := range group {
		out = append(out, participantMissing{
			participant:     participant,
			missingFragment: union.renderMissingFragment(c.operation, c.definition, participant.selectionSetRef, typeNode, ds),
		})
	}

	return out, true
}

func (c *nodeSelectionVisitor) mergeRequestScopedRequiresSelectionSet(union *requestScopedSelectionUnion, doc *ast.Document, selectionSetRef int, enclosingTypeNode ast.Node, ds DataSource) bool {
	enclosingTypeName := enclosingTypeNode.NameString(c.definition)

	for _, selectionRef := range doc.SelectionSets[selectionSetRef].SelectionRefs {
		if doc.Selections[selectionRef].Kind != ast.SelectionKindField {
			return false
		}

		fieldRef := doc.Selections[selectionRef].Ref
		fieldName := doc.FieldNameString(fieldRef)
		if !fieldBelongsToDataSource(ds, enclosingTypeName, fieldName) {
			continue
		}

		requiresConfiguration, exists := c.requiresConfigurationForField(ds, enclosingTypeName, fieldName)
		if exists {
			requiredFieldsDoc, report := RequiredFieldsFragment(requiresConfiguration.TypeName, requiresConfiguration.SelectionSet, false)
			if report.HasErrors() || len(requiredFieldsDoc.FragmentDefinitions) == 0 {
				return false
			}

			requiredSelectionSetRef := requiredFieldsDoc.FragmentDefinitions[0].SelectionSet
			if !union.mergeHiddenSelectionSet(requiredFieldsDoc, c.definition, requiredSelectionSetRef, enclosingTypeNode, ds) {
				return false
			}
			if !c.mergeRequestScopedRequiresSelectionSet(union, requiredFieldsDoc, requiredSelectionSetRef, enclosingTypeNode, ds) {
				return false
			}
		}

		fieldSelectionSetRef, hasSelectionSet := doc.FieldSelectionSet(fieldRef)
		if !hasSelectionSet {
			continue
		}

		fieldTypeNode, ok := fieldTypeNodeForSelection(c.definition, enclosingTypeNode, fieldRef, doc.FieldNameBytes(fieldRef))
		if !ok {
			return false
		}
		if !c.mergeRequestScopedRequiresSelectionSet(union, doc, fieldSelectionSetRef, fieldTypeNode, ds) {
			return false
		}
	}

	return true
}

func newRequestScopedSelectionUnion() *requestScopedSelectionUnion {
	return &requestScopedSelectionUnion{
		variants:         make(map[string]*requestScopedUnionVariant),
		responseKeyIndex: make(map[string]map[string]struct{}),
	}
}

func (u *requestScopedSelectionUnion) mergeSelectionSet(doc, definition *ast.Document, selectionSetRef int, enclosingTypeNode ast.Node, ds DataSource) bool {
	for _, selectionRef := range doc.SelectionSets[selectionSetRef].SelectionRefs {
		if doc.Selections[selectionRef].Kind != ast.SelectionKindField {
			return false
		}

		fieldRef := doc.Selections[selectionRef].Ref
		fieldName := doc.FieldNameString(fieldRef)
		if !fieldBelongsToDataSource(ds, enclosingTypeNode.NameString(definition), fieldName) {
			continue
		}

		argsPrinted := printFieldArgumentsDeterministic(doc, fieldRef)
		directivesPrinted := printFieldDirectivesDeterministic(doc, fieldRef)
		responseKey := doc.FieldAliasOrNameString(fieldRef)
		variantKey := requestScopedVariantKey(fieldName, argsPrinted, directivesPrinted)

		fieldTypeNode, ok := fieldTypeNodeForSelection(definition, enclosingTypeNode, fieldRef, doc.FieldNameBytes(fieldRef))
		if !ok && doc.FieldHasSelections(fieldRef) {
			return false
		}

		existing, exists := u.variants[variantKey]
		if !exists {
			existing = &requestScopedUnionVariant{
				key:                  variantKey,
				schemaFieldName:      fieldName,
				argsPrinted:          argsPrinted,
				directivesPrinted:    directivesPrinted,
				observedResponseKeys: map[string]struct{}{responseKey: {}},
			}
			if fieldSelectionSetRef, ok := doc.FieldSelectionSet(fieldRef); ok {
				existing.subSelection = newRequestScopedSelectionUnion()
				if !existing.subSelection.mergeSelectionSet(doc, definition, fieldSelectionSetRef, fieldTypeNode, ds) {
					return false
				}
			}
			u.variants[variantKey] = existing
		} else {
			existing.observedResponseKeys[responseKey] = struct{}{}

			fieldSelectionSetRef, hasFieldSelectionSet := doc.FieldSelectionSet(fieldRef)
			if !hasFieldSelectionSet {
				if existing.subSelection != nil {
					return false
				}
			} else {
				if existing.subSelection == nil {
					return false
				}
				if !existing.subSelection.mergeSelectionSet(doc, definition, fieldSelectionSetRef, fieldTypeNode, ds) {
					return false
				}
			}
		}

		if _, ok := u.responseKeyIndex[responseKey]; !ok {
			u.responseKeyIndex[responseKey] = make(map[string]struct{})
		}
		u.responseKeyIndex[responseKey][variantKey] = struct{}{}
	}

	return true
}

func (u *requestScopedSelectionUnion) mergeHiddenSelectionSet(doc, definition *ast.Document, selectionSetRef int, enclosingTypeNode ast.Node, ds DataSource) bool {
	for _, selectionRef := range doc.SelectionSets[selectionSetRef].SelectionRefs {
		if doc.Selections[selectionRef].Kind != ast.SelectionKindField {
			return false
		}

		fieldRef := doc.Selections[selectionRef].Ref
		fieldName := doc.FieldNameString(fieldRef)
		if !fieldBelongsToDataSource(ds, enclosingTypeNode.NameString(definition), fieldName) {
			continue
		}

		argsPrinted := printFieldArgumentsDeterministic(doc, fieldRef)
		directivesPrinted := printFieldDirectivesDeterministic(doc, fieldRef)
		responseKey := doc.FieldAliasOrNameString(fieldRef)
		variantKey := requestScopedVariantKey(fieldName, argsPrinted, directivesPrinted)

		fieldTypeNode, ok := fieldTypeNodeForSelection(definition, enclosingTypeNode, fieldRef, doc.FieldNameBytes(fieldRef))
		if !ok && doc.FieldHasSelections(fieldRef) {
			return false
		}

		existing, exists := u.variants[variantKey]
		if !exists {
			existing = &requestScopedUnionVariant{
				key:                  variantKey,
				schemaFieldName:      fieldName,
				argsPrinted:          argsPrinted,
				directivesPrinted:    directivesPrinted,
				observedResponseKeys: map[string]struct{}{responseKey: {}},
			}
			if fieldSelectionSetRef, ok := doc.FieldSelectionSet(fieldRef); ok {
				existing.subSelection = newRequestScopedSelectionUnion()
				if !existing.subSelection.mergeHiddenSelectionSet(doc, definition, fieldSelectionSetRef, fieldTypeNode, ds) {
					return false
				}
			}
			u.variants[variantKey] = existing

			if _, ok := u.responseKeyIndex[responseKey]; !ok {
				u.responseKeyIndex[responseKey] = make(map[string]struct{})
			}
			u.responseKeyIndex[responseKey][variantKey] = struct{}{}
			continue
		}

		fieldSelectionSetRef, hasFieldSelectionSet := doc.FieldSelectionSet(fieldRef)
		if !hasFieldSelectionSet {
			if existing.subSelection != nil {
				return false
			}
			continue
		}
		if existing.subSelection == nil {
			return false
		}
		if !existing.subSelection.mergeHiddenSelectionSet(doc, definition, fieldSelectionSetRef, fieldTypeNode, ds) {
			return false
		}
	}

	return true
}

func (u *requestScopedSelectionUnion) renderMissingFragment(doc, definition *ast.Document, selectionSetRef int, enclosingTypeNode ast.Node, ds DataSource) string {
	snapshot := buildRequestScopedSelectionSnapshot(doc, definition, selectionSetRef, enclosingTypeNode, ds)
	syntheticAliases := u.syntheticAliases()

	parts := make([]string, 0, len(u.variants))
	for _, variantKey := range u.sortedVariantKeys() {
		variant := u.variants[variantKey]
		fieldRef, exists := snapshot.fieldRefsByVariantKey[variantKey]
		if !exists {
			responseKey := variant.preferredResponseKey()
			if synthetic, ok := syntheticAliases[variantKey]; ok {
				responseKey = synthetic
			}
			parts = append(parts, variant.render(responseKey))
			continue
		}

		if variant.subSelection == nil {
			continue
		}

		fieldSelectionSetRef, ok := doc.FieldSelectionSet(fieldRef)
		if !ok {
			continue
		}

		fieldTypeNode, ok := fieldTypeNodeForSelection(definition, enclosingTypeNode, fieldRef, doc.FieldNameBytes(fieldRef))
		if !ok {
			continue
		}

		subMissing := variant.subSelection.renderMissingFragment(doc, definition, fieldSelectionSetRef, fieldTypeNode, ds)
		if subMissing == "" {
			continue
		}

		parts = append(parts, renderFieldWithExistingResponseKey(doc, fieldRef, subMissing))
	}

	return strings.Join(parts, " ")
}

func (u *requestScopedSelectionUnion) recordExistingSelectionAliases(doc, definition *ast.Document, selectionSetRef int, enclosingTypeNode ast.Node, ds DataSource, syntheticAliases map[string]string, visibleResponseKeys map[int]string, fetchAliases map[int]string) bool {
	for _, selectionRef := range doc.SelectionSets[selectionSetRef].SelectionRefs {
		if doc.Selections[selectionRef].Kind != ast.SelectionKindField {
			return false
		}

		fieldRef := doc.Selections[selectionRef].Ref
		fieldName := doc.FieldNameString(fieldRef)
		if !fieldBelongsToDataSource(ds, enclosingTypeNode.NameString(definition), fieldName) {
			continue
		}

		argsPrinted := printFieldArgumentsDeterministic(doc, fieldRef)
		directivesPrinted := printFieldDirectivesDeterministic(doc, fieldRef)
		responseKey := doc.FieldAliasOrNameString(fieldRef)
		variantKey := requestScopedVariantKey(fieldName, argsPrinted, directivesPrinted)
		variant, ok := u.variants[variantKey]
		if !ok {
			continue
		}

		if syntheticAlias, hasSyntheticAlias := syntheticAliases[variantKey]; hasSyntheticAlias && responseKey != syntheticAlias {
			if _, exists := visibleResponseKeys[fieldRef]; !exists {
				visibleResponseKeys[fieldRef] = responseKey
			}
			fetchAliases[fieldRef] = syntheticAlias
		}

		fieldSelectionSetRef, hasFieldSelectionSet := doc.FieldSelectionSet(fieldRef)
		if !hasFieldSelectionSet || variant.subSelection == nil {
			continue
		}

		fieldTypeNode, ok := fieldTypeNodeForSelection(definition, enclosingTypeNode, fieldRef, doc.FieldNameBytes(fieldRef))
		if !ok {
			return false
		}
		if !variant.subSelection.recordExistingSelectionAliases(doc, definition, fieldSelectionSetRef, fieldTypeNode, ds, variant.subSelection.syntheticAliases(), visibleResponseKeys, fetchAliases) {
			return false
		}
	}

	return true
}

func buildRequestScopedSelectionSnapshot(doc, definition *ast.Document, selectionSetRef int, enclosingTypeNode ast.Node, ds DataSource) requestScopedSelectionSnapshot {
	out := requestScopedSelectionSnapshot{
		fieldRefsByVariantKey: make(map[string]int),
		responseKeys:          make(map[string]struct{}),
	}

	for _, fieldRef := range doc.SelectionSetFieldRefs(selectionSetRef) {
		fieldName := doc.FieldNameString(fieldRef)
		if !fieldBelongsToDataSource(ds, enclosingTypeNode.NameString(definition), fieldName) {
			continue
		}

		argsPrinted := printFieldArgumentsDeterministic(doc, fieldRef)
		directivesPrinted := printFieldDirectivesDeterministic(doc, fieldRef)
		responseKey := doc.FieldAliasOrNameString(fieldRef)
		variantKey := requestScopedVariantKey(fieldName, argsPrinted, directivesPrinted)

		out.fieldRefsByVariantKey[variantKey] = fieldRef
		out.responseKeys[responseKey] = struct{}{}
	}

	return out
}

func requestScopedVariantKey(fieldName, argsPrinted, directivesPrinted string) string {
	return fieldName + "\x00" + argsPrinted + "\x00" + directivesPrinted
}

func (u *requestScopedSelectionUnion) syntheticAliases() map[string]string {
	out := make(map[string]string)
	reservedResponseKeys := make(map[string]struct{})
	for responseKey := range u.responseKeyIndex {
		reservedResponseKeys[responseKey] = struct{}{}
	}

	responseKeys := make([]string, 0, len(u.responseKeyIndex))
	for responseKey, variantKeys := range u.responseKeyIndex {
		if len(variantKeys) < 2 {
			continue
		}
		responseKeys = append(responseKeys, responseKey)
	}
	sort.Strings(responseKeys)

	for _, responseKey := range responseKeys {
		variantKeys := make([]string, 0, len(u.responseKeyIndex[responseKey]))
		for variantKey := range u.responseKeyIndex[responseKey] {
			variantKeys = append(variantKeys, variantKey)
		}
		sort.Strings(variantKeys)

		base := "__request_scoped__" + sanitizeGraphQLName(responseKey) + "_"
		for _, variantKey := range variantKeys {
			if existingAlias, ok := u.variants[variantKey].existingSyntheticAlias(base); ok {
				out[variantKey] = existingAlias
				reservedResponseKeys[existingAlias] = struct{}{}
			}
		}

		nextIndex := 0
		for _, variantKey := range variantKeys {
			if _, exists := out[variantKey]; exists {
				continue
			}
			for {
				candidate := base + strconvItoa(nextIndex)
				nextIndex++
				if _, exists := reservedResponseKeys[candidate]; exists {
					continue
				}
				reservedResponseKeys[candidate] = struct{}{}
				out[variantKey] = candidate
				break
			}
		}
	}

	return out
}

func (f *requestScopedUnionVariant) existingSyntheticAlias(base string) (string, bool) {
	keys := make([]string, 0, len(f.observedResponseKeys))
	for key := range f.observedResponseKeys {
		if strings.HasPrefix(key, base) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return "", false
	}
	sort.Strings(keys)
	return keys[0], true
}

func sanitizeGraphQLName(in string) string {
	if in == "" {
		return "field"
	}

	var out strings.Builder
	for i := 0; i < len(in); i++ {
		b := in[i]
		switch {
		case b >= 'a' && b <= 'z':
			out.WriteByte(b)
		case b >= 'A' && b <= 'Z':
			out.WriteByte(b)
		case b >= '0' && b <= '9':
			out.WriteByte(b)
		case b == '_':
			out.WriteByte(b)
		default:
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "field"
	}
	return out.String()
}

func (u *requestScopedSelectionUnion) sortedVariantKeys() []string {
	keys := make([]string, 0, len(u.variants))
	for key := range u.variants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (f *requestScopedUnionVariant) preferredResponseKey() string {
	if _, ok := f.observedResponseKeys[f.schemaFieldName]; ok {
		return f.schemaFieldName
	}
	keys := make([]string, 0, len(f.observedResponseKeys))
	for key := range f.observedResponseKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys[0]
}

func (f *requestScopedUnionVariant) render(responseKey string) string {
	selection := ""
	if f.subSelection != nil {
		selection = f.subSelection.renderCompleteSelection()
	}
	return renderFieldString(responseKey, f.schemaFieldName, f.argsPrinted, f.directivesPrinted, selection)
}

func (u *requestScopedSelectionUnion) renderCompleteSelection() string {
	parts := make([]string, 0, len(u.variants))
	for _, variantKey := range u.sortedVariantKeys() {
		variant := u.variants[variantKey]
		parts = append(parts, variant.render(variant.preferredResponseKey()))
	}
	return strings.Join(parts, " ")
}

func renderFieldWithExistingResponseKey(doc *ast.Document, fieldRef int, selection string) string {
	return renderFieldString(
		doc.FieldAliasOrNameString(fieldRef),
		doc.FieldNameString(fieldRef),
		printFieldArgumentsDeterministic(doc, fieldRef),
		printFieldDirectivesDeterministic(doc, fieldRef),
		selection,
	)
}

func renderFieldString(responseKey, schemaFieldName, argsPrinted, directivesPrinted, selection string) string {
	var prefix strings.Builder
	if responseKey != schemaFieldName {
		prefix.WriteString(responseKey)
		prefix.WriteString(": ")
	}
	prefix.WriteString(schemaFieldName)
	prefix.WriteString(argsPrinted)
	if directivesPrinted != "" {
		prefix.WriteByte(' ')
		prefix.WriteString(directivesPrinted)
	}
	if selection == "" {
		return prefix.String()
	}
	prefix.WriteString(" { ")
	prefix.WriteString(selection)
	prefix.WriteString(" }")
	return prefix.String()
}

func printFieldArgumentsDeterministic(doc *ast.Document, fieldRef int) string {
	if !doc.FieldHasArguments(fieldRef) {
		return ""
	}

	refs := append([]int(nil), doc.FieldArguments(fieldRef)...)
	sort.Slice(refs, func(i, j int) bool {
		return doc.ArgumentNameString(refs[i]) < doc.ArgumentNameString(refs[j])
	})

	var out strings.Builder
	_ = doc.PrintArguments(refs, &out)
	return out.String()
}

func printFieldDirectivesDeterministic(doc *ast.Document, fieldRef int) string {
	if !doc.FieldHasDirectives(fieldRef) {
		return ""
	}

	refs := append([]int(nil), doc.FieldDirectives(fieldRef)...)
	sort.Slice(refs, func(i, j int) bool {
		leftName := doc.DirectiveNameString(refs[i])
		rightName := doc.DirectiveNameString(refs[j])
		if leftName == rightName {
			return printDirectiveDeterministic(doc, refs[i]) < printDirectiveDeterministic(doc, refs[j])
		}
		return leftName < rightName
	})

	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, printDirectiveDeterministic(doc, ref))
	}
	return strings.Join(parts, " ")
}

func printDirectiveDeterministic(doc *ast.Document, directiveRef int) string {
	directive := doc.Directives[directiveRef]
	out := "@" + doc.DirectiveNameString(directiveRef)
	if !directive.HasArguments {
		return out
	}

	refs := append([]int(nil), directive.Arguments.Refs...)
	sort.Slice(refs, func(i, j int) bool {
		return doc.ArgumentNameString(refs[i]) < doc.ArgumentNameString(refs[j])
	})

	var args strings.Builder
	_ = doc.PrintArguments(refs, &args)
	return out + args.String()
}

func (c *nodeSelectionVisitor) dataSourceByHash(hash DSHash) (DataSource, bool) {
	for _, ds := range c.dataSources {
		if ds.Hash() == hash {
			return ds, true
		}
	}
	return nil, false
}

func fieldTypeNodeForSelection(definition *ast.Document, enclosingTypeNode ast.Node, fieldRef int, fieldName []byte) (ast.Node, bool) {
	fieldDefinitionRef, ok := definition.NodeFieldDefinitionByName(enclosingTypeNode, fieldName)
	if !ok {
		return ast.InvalidNode, false
	}
	return definition.Index.FirstNodeByNameStr(definition.FieldDefinitionTypeNameString(fieldDefinitionRef))
}

func fieldBelongsToDataSource(ds DataSource, typeName, fieldName string) bool {
	if fieldName == typeNameField {
		return ds.HasRootNodeWithTypename(typeName) || ds.HasChildNodeWithTypename(typeName)
	}
	return ds.HasRootNode(typeName, fieldName) || ds.HasChildNode(typeName, fieldName)
}

func strconvItoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits [20]byte
	pos := len(digits)
	for i > 0 {
		pos--
		digits[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(digits[pos:])
}
