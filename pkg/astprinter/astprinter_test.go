package astprinter

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"testing"
)

func TestPrint(t *testing.T) {

	run := func(raw string, want string) {

		in := &input.Input{}
		in.ResetInputBytes([]byte(raw))

		doc := ast.NewDocument()

		parser := astparser.NewParser()
		err := parser.Parse(in, doc)
		if err != nil {
			panic(err)
		}

		buff := &bytes.Buffer{}

		printer := Printer{}

		printer.SetInput(doc, in)
		err = printer.Print(buff)
		if err != nil {
			panic(err)
		}

		got := buff.String()

		if want != got {
			panic(fmt.Errorf("want:\n%s\ngot:\n%s\n", want, got))
		}
	}

	t.Run("complex", func(t *testing.T) {
		run(`	
				subscription sub {
					...multipleSubscriptions
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`,
			"subscription sub {...multipleSubscriptions} fragment multipleSubscriptions on Subscription {newMessage {body sender} disallowedSecondRootField}")
	})
}

func BenchmarkPrint(b *testing.B) {

	run := func(b *testing.B, raw string, want string) {

		in := &input.Input{}
		in.ResetInputBytes([]byte(raw))

		doc := ast.NewDocument()

		parser := astparser.NewParser()
		err := parser.Parse(in, doc)
		if err != nil {
			panic(err)
		}

		buff := bytes.NewBuffer(make([]byte, 1024))

		printer := Printer{}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buff.Reset()
			printer.SetInput(doc, in)
			printer.Print(buff)
		}
	}

	b.Run("complex", func(b *testing.B) {
		run(b, `	
				subscription sub {
					...multipleSubscriptions
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`,
			"subscription sub {...multipleSubscriptions} fragment multipleSubscriptions on Subscription {newMessage {body sender} disallowedSecondRootField}")
	})
}
