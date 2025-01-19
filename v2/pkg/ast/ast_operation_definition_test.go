package ast_test

import (
	"fmt"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestDocument_OperationNameExists(t *testing.T) {
	run := func(schema string, operationName string, expectedExists bool) func(t *testing.T) {
		return func(t *testing.T) {
			doc := unsafeparser.ParseGraphqlDocumentString(schema)
			exists := doc.OperationNameExists(operationName)
			assert.Equal(t, expectedExists, exists)
		}
	}

	t.Run("not found on empty document", run(
		"",
		"MyOperation",
		false,
	))

	t.Run("not found on document with multiple operations", run(
		"query OtherOperation {other} query AnotherOperation {another}",
		"MyOperation",
		false,
	))

	t.Run("found on document with a single operations", run(
		"query MyOperation {}",
		"MyOperation",
		true,
	))

	t.Run("found on document with multiple operations", run(
		"query OtherOperation {other} query AnotherOperation {another} query MyOperation {}",
		"MyOperation",
		true,
	))

	t.Run("found on a document with preceding root nodes of not operation type", run(
		"fragment F on T {field} query MyOperation {}",
		"MyOperation",
		true,
	))

	t.Run("LetterIndices.Increment works correctly", func(t *testing.T) {
		input := ast.NewLetterIndices([]int{0, 25}, []byte{'a', 'z'})
		input.Increment()
		assert.Equal(t, ast.NewLetterIndices([]int{1, 0}, []byte{'b', 'a'}), input)
		input.Increment()
		assert.Equal(t, ast.NewLetterIndices([]int{1, 1}, []byte{'b', 'b'}), input)
		input = ast.NewLetterIndices([]int{1, 25}, []byte{'b', 'z'})
		input.Increment()
		assert.Equal(t, ast.NewLetterIndices([]int{2, 0}, []byte{'c', 'a'}), input)
		input = ast.NewLetterIndices([]int{25, 25}, []byte{'z', 'z'})
		input.Increment()
		assert.Equal(t, ast.NewLetterIndices([]int{0, 0, 0}, []byte{'a', 'a', 'a'}), input)
	})

	t.Run("schema string is generated correctly #1", func(t *testing.T) {
		assert.Equal(t,
			`query ($a: Int! $b: Int! $c: Int! $d: Int! $e: Int! $f: Int! $g: Int! $h: Int! $i: Int! $j: Int! $k: Int! $l: Int! $m: Int! $n: Int! $o: Int! $p: Int! $q: Int! $r: Int! $s: Int! $t: Int! $u: Int! $v: Int! $w: Int! $x: Int! $y: Int! $z: Int! $aa: Int! $ab: Int!) {
	fielda(arg: $a)
	fieldb(arg: $b)
	fieldc(arg: $c)
	fieldd(arg: $d)
	fielde(arg: $e)
	fieldf(arg: $f)
	fieldg(arg: $g)
	fieldh(arg: $h)
	fieldi(arg: $i)
	fieldj(arg: $j)
	fieldk(arg: $k)
	fieldl(arg: $l)
	fieldm(arg: $m)
	fieldn(arg: $n)
	fieldo(arg: $o)
	fieldp(arg: $p)
	fieldq(arg: $q)
	fieldr(arg: $r)
	fields(arg: $s)
	fieldt(arg: $t)
	fieldu(arg: $u)
	fieldv(arg: $v)
	fieldw(arg: $w)
	fieldx(arg: $x)
	fieldy(arg: $y)
	fieldz(arg: $z)
	fieldaa(arg: $aa)
	fieldab(arg: $ab)
}`,
			schemaString(28))
	})

	t.Run("test that schema string is generated correctly #2", func(t *testing.T) {
		assert.True(t, strings.HasSuffix(schemaString(704), "fieldzy(arg: $zy)\n\tfieldzz(arg: $zz)\n\tfieldaaa(arg: $aaa)\n\tfieldaab(arg: $aab)\n}"))
	})

	t.Run("test that schema string is generated correctly #3", func(t *testing.T) {
		assert.True(t, strings.HasSuffix(schemaString(18280), "fieldzzy(arg: $zzy)\n\tfieldzzz(arg: $zzz)\n\tfieldaaaa(arg: $aaaa)\n\tfieldaaab(arg: $aaab)\n}"))
	})

	t.Run("next variable #1", func(t *testing.T) {
		op := unsafeparser.ParseGraphqlDocumentString(schemaString(1))
		assert.Equal(t, "b", string(op.GenerateUnusedVariableDefinitionNameV2(0)))
	})

	t.Run("next variable #2", func(t *testing.T) {
		op := unsafeparser.ParseGraphqlDocumentString(schemaString(26))
		assert.Equal(t, "aa", string(op.GenerateUnusedVariableDefinitionNameV2(0)))
	})

	t.Run("next variable #3", func(t *testing.T) {
		op := unsafeparser.ParseGraphqlDocumentString(schemaString(702))
		assert.Equal(t, "aaa", string(op.GenerateUnusedVariableDefinitionNameV2(0)))
	})

	t.Run("next variable #4", func(t *testing.T) {
		op := unsafeparser.ParseGraphqlDocumentString(schemaString(18278))
		assert.Equal(t, "aaaa", string(op.GenerateUnusedVariableDefinitionNameV2(0)))
	})
}

func schemaString(varNumber int) string {
	vars := make([]string, varNumber)
	out := make([]string, varNumber)
	l := ast.NewDefaultLetterIndices()
	for i := 0; i < varNumber; i++ {
		varName := l.Render()
		out[i] = fmt.Sprintf("	field%s(arg: $%s)", varName, varName)
		vars[i] = fmt.Sprintf("$%s: Int!", varName)
		l.Increment()
	}
	prefix := "query (" + fmt.Sprintf(strings.Join(vars, " ")) + ") {\n"
	return prefix + strings.Join(out, "\n") + "\n}"
}
