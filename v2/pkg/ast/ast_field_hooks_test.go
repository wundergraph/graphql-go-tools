package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestDocumentFieldHooks(t *testing.T) {
	t.Run("OnCopyField is called with the source and the copied field ref", func(t *testing.T) {
		doc := ast.NewDocument()
		fieldRef := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref

		var gotFrom, gotTo int
		calls := 0
		doc.OnCopyField = func(fieldRef, copyRef int) {
			gotFrom, gotTo = fieldRef, copyRef
			calls++
		}

		copyRef := doc.CopyField(fieldRef)

		assert.Equal(t, 1, calls)
		assert.Equal(t, fieldRef, gotFrom)
		assert.Equal(t, copyRef, gotTo)
		assert.NotEqual(t, fieldRef, copyRef)
	})

	t.Run("CopyField without hook does not panic", func(t *testing.T) {
		doc := ast.NewDocument()
		fieldRef := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref

		copyRef := doc.CopyField(fieldRef)
		assert.NotEqual(t, fieldRef, copyRef)
	})

	t.Run("OnMergeFields is called with the survivor and the removed field ref", func(t *testing.T) {
		doc := ast.NewDocument()
		left := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref
		right := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref

		var gotSurvivor, gotRemoved int
		calls := 0
		doc.OnMergeFields = func(survivorRef, removedRef int) {
			gotSurvivor, gotRemoved = survivorRef, removedRef
			calls++
		}

		doc.MergeFieldsDefer(left, right)

		assert.Equal(t, 1, calls)
		assert.Equal(t, left, gotSurvivor)
		assert.Equal(t, right, gotRemoved)
	})

	t.Run("Reset clears the hooks", func(t *testing.T) {
		doc := ast.NewDocument()
		doc.OnCopyField = func(fieldRef, copyRef int) {}
		doc.OnMergeFields = func(survivorRef, removedRef int) {}

		doc.Reset()

		assert.Nil(t, doc.OnCopyField)
		assert.Nil(t, doc.OnMergeFields)
	})
}
