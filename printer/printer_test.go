package printer

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"testing"
)

func TestPrinter(t *testing.T) {

	run := func(input string) {

		inputBytes := []byte(input)

		p := parser.NewParser()
		err := p.ParseExecutableDefinition(inputBytes)
		if err != nil {
			panic(err)
		}

		printer := New()
		printer.SetInput(p)

		buff := bytes.Buffer{}
		out := bufio.NewWriter(&buff)
		err = printer.PrintExecutableSchema(out)
		if err != nil {
			panic(err)
		}

		err = out.Flush()
		if err != nil {
			panic(err)
		}

		printedBytes := buff.Bytes()
		if !bytes.Equal(printedBytes, inputBytes) {
			panic(fmt.Errorf("want:\n\n%s\n\ngot:\n\n%s\n", string(inputBytes), string(printedBytes)))
		}
	}

	t.Run("single field", func(t *testing.T) {
		run("{foo}")
	})
	t.Run("two fields", func(t *testing.T) {
		run("{foo bar}")
	})
	t.Run("field with subselection", func(t *testing.T) {
		run("{foo {bar}}")
	})
	t.Run("fields with spread and inline", func(t *testing.T) {
		run("{foo {bar {bat ...bal ...{bak}}} baz}")
	})
	t.Run("inline fragment with type condition", func(t *testing.T) {
		run("{foo ...on Bar {baz}}")
	})
	t.Run("field with fragment spread", func(t *testing.T) {
		run("{foo ...Bar}")
	})
	t.Run("complex", func(t *testing.T) {
		run("{foo bar ...{baz} ...Bal ...on Bar {bat bar} bart}")
	})
	t.Run("field with arguments", func(t *testing.T) {
		run("{assets(first:1) noArgField}")
	})
	t.Run("null arg", func(t *testing.T) {
		run("{assets(first:null)}")
	})
	t.Run("enum arg", func(t *testing.T) {
		run("{assets(first:ENUM)}")
	})
	t.Run("true arg", func(t *testing.T) {
		run("{assets(first:true)}")
	})
	t.Run("false arg", func(t *testing.T) {
		run("{assets(first:false)}")
	})
	t.Run("integer arg", func(t *testing.T) {
		run("{assets(first:1337)}")
	})
	t.Run("float arg", func(t *testing.T) {
		run("{assets(first:13.37)}")
	})
	t.Run("string arg", func(t *testing.T) {
		run("{assets(first:\"foo\")}")
	})
	t.Run("variable arg", func(t *testing.T) {
		run("{assets(first:$foo)}")
	})
	t.Run("object arg", func(t *testing.T) {
		run("{assets(first:{foo:\"bar\",baz:1})}")
	})
	t.Run("list arg", func(t *testing.T) {
		run("{assets(first:[1,3,3,7])}")
	})
}

func BenchmarkPrinter_PrintExecutableSchema(b *testing.B) {

	inputBytes := []byte("{foo bar ...{baz} ...Bal ...on Bar {bat bar} bart assets(first:{foo:\"bar\",baz:1}) assets(first:[1,3,3,7]) assets(first:null)}")

	p := parser.NewParser()
	err := p.ParseExecutableDefinition(inputBytes)
	if err != nil {
		panic(err)
	}

	printer := New()

	buff := bytes.Buffer{}
	bufOut := bufio.NewWriter(&buff)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		printer.SetInput(p)
		err = printer.PrintExecutableSchema(bufOut)
		if err != nil {
			panic(err)
		}

		if err := bufOut.Flush(); err != nil {
			panic(err)
		}

		printedBytes := buff.Bytes()
		if !bytes.Equal(printedBytes, inputBytes) {
			panic(fmt.Errorf("want:\n\n%s\n\ngot:\n\n%s\n", string(inputBytes), string(printedBytes)))
		}

		buff.Reset()
		bufOut.Reset(&buff)
	}
}
