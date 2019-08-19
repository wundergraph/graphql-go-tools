package astinspect

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

func FieldsCanMerge(document *ast.Document, left, right int) bool {
	leftName := document.FieldName(left)
	rightName := document.FieldName(right)
	leftAlias := document.FieldAlias(left)
	rightAlias := document.FieldAlias(right)

	/*	_leftName := string(leftName)
		_rightName := string(rightName)
		_leftAlias := string(leftAlias)
		_rightAlias := string(rightAlias)
		_, _, _, _ = _leftName, _leftAlias, _rightName, _rightAlias*/

	leftHasAlias := document.Fields[left].Alias.IsDefined
	rightHasAlias := document.Fields[right].Alias.IsDefined
	noAlias := !leftHasAlias && !rightHasAlias

	if noAlias && !bytes.Equal(leftName, rightName) {
		return true
	}

	if leftHasAlias && !rightHasAlias {
		return !bytes.Equal(leftAlias, rightName)
	}

	if rightHasAlias && !leftHasAlias {
		return !bytes.Equal(rightAlias, leftName)
	}

	if bytes.Equal(leftAlias, rightAlias) {
		return document.FieldsAreEqualFlat(left, right)
	}

	return true
}
