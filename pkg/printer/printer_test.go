package printer

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/sebdah/goldie"
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

		l := lookup.New(p, 256)
		l.SetParser(p)

		w := lookup.NewWalker(1024, 8)
		w.SetLookup(l)
		w.WalkExecutable()

		printer := New()
		printer.SetInput(p, l, w)

		buff := bytes.Buffer{}
		out := bufio.NewWriter(&buff)
		printer.PrintExecutableSchema(out)
		if printer.err != nil {
			panic(printer.err)
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
	t.Run("query prefix", func(t *testing.T) {
		run("query MyQuery {foo}")
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
		run("{foo ...on Bar{baz}}")
	})
	t.Run("inline fragment with type condition and directive", func(t *testing.T) {
		run("{foo ...on Bar @foo @bar(baz:\"bat\"){baz}}")
	})
	t.Run("field with fragment spread", func(t *testing.T) {
		run("{foo ...Bar}")
	})
	t.Run("field with fragment spread and directive", func(t *testing.T) {
		run("{foo ...Bar @foo}")
	})
	t.Run("complex", func(t *testing.T) {
		run("{foo bar ...{baz} ...Bal ...on Bar{bat bar} bart}")
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
	t.Run("fragment definition", func(t *testing.T) {
		run("fragment MyFragment on Dog {foo bar}")
	})
	t.Run("fragment definition with directive", func(t *testing.T) {
		run("fragment MyFragment on Dog @foo @bar(baz:\"bat\") {foo bar}")
	})
	t.Run("multiple fragment definitions", func(t *testing.T) {
		run("fragment MyFragment on Dog {foo bar}\nfragment MyFragment on Dog {foo bar}")
	})
	t.Run("directive on query", func(t *testing.T) {
		run("query mQuery @foo(bar:\"baz\") {bat}")
	})
	t.Run("multiple directives on query", func(t *testing.T) {
		run("query mQuery @foo(bar:\"baz\") @foo2 {bat}")
	})
	t.Run("directive on field", func(t *testing.T) {
		run("{foo @bar(baz:\"bat\")}")
	})
	t.Run("multiple directive on field", func(t *testing.T) {
		run("{foo @bar(baz:\"bat\") @foo2}")
	})
}

func TestPrinter_Regression(t *testing.T) {
	inputBytes := []byte(introspectionQuery)

	p := parser.NewParser()
	err := p.ParseExecutableDefinition(inputBytes)
	if err != nil {
		panic(err)
	}

	l := lookup.New(p, 256)
	w := lookup.NewWalker(1024, 8)
	w.SetLookup(l)
	w.WalkExecutable()

	printer := New()
	printer.SetInput(p, l, w)

	buff := bytes.Buffer{}
	out := bufio.NewWriter(&buff)
	printer.PrintExecutableSchema(out)
	if printer.err != nil {
		panic(printer.err)
	}

	err = out.Flush()
	if err != nil {
		panic(err)
	}

	printedBytes := buff.Bytes()
	goldie.Assert(t, "printer_introspection", printedBytes)
}

func BenchmarkPrinter_PrintExecutableSchema(b *testing.B) {

	inputBytes := []byte("{foo bar ...{baz} ...Bal ...on Bar{bat bar} bart assets(first:{foo:\"bar\",baz:1}) assets(first:[1,3,3,7]) assets(first:null)}\nfragment MyFrag on Dog {foo bar}")

	p := parser.NewParser()
	err := p.ParseExecutableDefinition(inputBytes)
	if err != nil {
		panic(err)
	}

	printer := New()

	buff := bytes.Buffer{}
	bufOut := bufio.NewWriter(&buff)

	l := lookup.New(p, 256)
	w := lookup.NewWalker(1024, 8)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		w.SetLookup(l)
		w.WalkExecutable()

		printer.SetInput(p, l, w)
		printer.PrintExecutableSchema(bufOut)
		if printer.err != nil {
			panic(printer.err)
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

var introspectionQuery = `query IntrospectionQuery {
  __schema {
    queryType {
      name
    }
    mutationType {
      name
    }
    subscriptionType {
      name
    }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`
