package cacheconfig

import (
	"fmt"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
)

// The SDL bridge of the per-type tier: composition carries the declaration
// through to the router, and the router turns the subgraph's SDL into the
// SubgraphCacheConfig.Types map with the reader below. Nothing here runs at
// plan or request time.

// CacheDirectiveDefinition is the SDL the per-type declaration is written in —
// the directive and the enum its scope argument takes. Composition tooling
// references it so subgraph authors and this reader agree on one spelling.
const CacheDirectiveDefinition = `enum CacheScope {
  PUBLIC
  PRIVATE
}

directive @cache(maxAge: Int!, scope: CacheScope = PUBLIC) on OBJECT`

const (
	cacheDirectiveName = "cache"
	maxAgeArgumentName = "maxAge"
	scopeArgumentName  = "scope"
)

// ExtractTypeCacheConfigs reads the @cache declarations of ONE subgraph SDL
// into the per-type tier of the cascade, keyed by type name. Declarations are
// read off object type definitions and object type extensions alike, since a
// subgraph's contribution to a shared entity is usually an extension.
//
// The extraction never fails: a declaration whose arguments do not fit the
// directive is skipped and named in the returned warnings, which the caller
// surfaces to the operator; the remaining types are still extracted. maxAge is
// in SECONDS, and a type declared twice keeps its first declaration.
func ExtractTypeCacheConfigs(sdl string) (map[string]TypeCacheConfig, []string) {
	types := make(map[string]TypeCacheConfig)
	doc, report := astparser.ParseGraphqlDocumentString(sdl)
	if report.HasErrors() {
		return types, []string{"no @cache declaration was read, the SDL does not parse: " + report.Error()}
	}

	var warnings []string
	collect := func(typeName string, directiveRefs []int) {
		declaration, warning, ok := readCacheDeclaration(&doc, typeName, directiveRefs)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if !ok {
			return
		}
		if _, duplicate := types[typeName]; duplicate {
			warnings = append(warnings, fmt.Sprintf("type %q: a repeated @cache declaration was ignored", typeName))
			return
		}
		types[typeName] = declaration
	}
	for ref := range doc.ObjectTypeDefinitions {
		collect(doc.ObjectTypeDefinitionNameString(ref), doc.ObjectTypeDefinitions[ref].Directives.Refs)
	}
	for ref := range doc.ObjectTypeExtensions {
		collect(doc.ObjectTypeExtensionNameString(ref), doc.ObjectTypeExtensions[ref].Directives.Refs)
	}
	return types, warnings
}

// readCacheDeclaration reads one type's @cache declaration. ok=false means the
// type declares nothing usable, and the returned warning is non-empty exactly
// when a declaration WAS found and skipped — an unusable one never falls back
// to a guessed value, because guessing PUBLIC for an unreadable scope would
// publish data the schema meant to keep private.
func readCacheDeclaration(doc *ast.Document, typeName string, directiveRefs []int) (TypeCacheConfig, string, bool) {
	directiveRef, exists := doc.DirectiveWithNameBytes(directiveRefs, []byte(cacheDirectiveName))
	if !exists {
		return TypeCacheConfig{}, "", false
	}

	maxAge, hasMaxAge := doc.DirectiveArgumentValueByName(directiveRef, []byte(maxAgeArgumentName))
	if !hasMaxAge || maxAge.Kind != ast.ValueKindInteger || !doc.IntValueValidInt32(maxAge.Ref) || doc.IntValueIsNegative(maxAge.Ref) {
		return TypeCacheConfig{}, fmt.Sprintf("type %q: @cache was skipped, its maxAge is not a non-negative Int", typeName), false
	}
	declaration := TypeCacheConfig{MaxAge: time.Duration(doc.IntValueAsInt32(maxAge.Ref)) * time.Second}

	scope, hasScope := doc.DirectiveArgumentValueByName(directiveRef, []byte(scopeArgumentName))
	if !hasScope {
		return declaration, "", true
	}
	if scope.Kind != ast.ValueKindEnum {
		return TypeCacheConfig{}, fmt.Sprintf("type %q: @cache was skipped, its scope is not a CacheScope value", typeName), false
	}
	switch doc.EnumValueNameString(scope.Ref) {
	case CacheScopePublic.String():
		// PUBLIC is the absence of privacy and the zero value already carries it.
	case CacheScopePrivate.String():
		declaration.Scope = CacheScopePrivate
	default:
		return TypeCacheConfig{}, fmt.Sprintf("type %q: @cache was skipped, its scope %q is not a CacheScope value",
			typeName, doc.EnumValueNameString(scope.Ref)), false
	}
	return declaration, "", true
}
