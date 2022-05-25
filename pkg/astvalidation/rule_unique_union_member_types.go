package astvalidation

import (
	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type hashedMembers map[uint64]bool

type uniqueUnionMemberTypesVisitor struct {
	*astvisitor.Walker
	definition       *ast.Document
	currentUnionName ast.ByteSlice
	currentUnionHash uint64
	presentMembers   map[uint64]hashedMembers
}

func UniqueUnionMemberTypes() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := &uniqueUnionMemberTypesVisitor{
			Walker: walker,
		}

		walker.RegisterEnterDocumentVisitor(visitor)
		walker.RegisterEnterUnionTypeDefinitionVisitor(visitor)
		walker.RegisterEnterUnionMemberTypeVisitor(visitor)
		walker.RegisterUnionTypeDefinitionVisitor(visitor)
		walker.RegisterUnionTypeExtensionVisitor(visitor)
	}
}

func (u *uniqueUnionMemberTypesVisitor) EnterDocument(operation, _ *ast.Document) {
	u.definition = operation
	u.currentUnionName = u.currentUnionName[:0]
	u.currentUnionHash = 0
	u.presentMembers = make(map[uint64]hashedMembers)
}

func (u *uniqueUnionMemberTypesVisitor) EnterUnionTypeDefinition(ref int) {
	unionName := u.definition.UnionTypeDefinitionNameBytes(ref)
	u.setCurrentUnion(unionName)
}

func (u *uniqueUnionMemberTypesVisitor) LeaveUnionTypeDefinition(_ int) {
	u.unsetCurrentUnion()
}

func (u *uniqueUnionMemberTypesVisitor) EnterUnionMemberType(ref int) {
	memberName := u.definition.TypeNameBytes(ref)
	u.checkMemberName(memberName)
}

func (u *uniqueUnionMemberTypesVisitor) EnterUnionTypeExtension(ref int) {
	unionName := u.definition.UnionTypeExtensionNameBytes(ref)
	u.setCurrentUnion(unionName)
}

func (u *uniqueUnionMemberTypesVisitor) LeaveUnionTypeExtension(_ int) {
	u.unsetCurrentUnion()
}

func (u *uniqueUnionMemberTypesVisitor) setCurrentUnion(unionName ast.ByteSlice) {
	u.currentUnionName = unionName
	u.currentUnionHash = xxhash.Sum64(unionName)
}

func (u *uniqueUnionMemberTypesVisitor) unsetCurrentUnion() {
	u.currentUnionName = u.currentUnionName[:0]
	u.currentUnionHash = 0
}

func (u *uniqueUnionMemberTypesVisitor) checkMemberName(memberName ast.ByteSlice) {
	if len(u.currentUnionName) == 0 || u.currentUnionHash == 0 {
		return
	}

	memberNameHash := xxhash.Sum64(memberName)
	memberNames, ok := u.presentMembers[u.currentUnionHash]
	if !ok {
		memberNames = make(hashedMembers)
	}

	if memberNames[memberNameHash] {
		u.Report.AddExternalError(operationreport.ErrUnionMembersMustBeUnique(u.currentUnionName, memberName))
		return
	}

	memberNames[memberNameHash] = true
	u.presentMembers[u.currentUnionHash] = memberNames
}
