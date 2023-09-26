package escape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytes(t *testing.T) {
	t.Run("", func(t *testing.T) {
		input := `foo
	bar
  baz	bal
"str"
`

		marshalled, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}

		want := marshalled[1 : len(marshalled)-1]

		var out []byte

		got := Bytes([]byte(input), out)
		if !bytes.Equal(got, want) {
			t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)", want, len(want), got, len(got), string(want), string(got))
		}

		out = make([]byte, len(input))

		got = Bytes([]byte(input), out)
		if !bytes.Equal(got, want) {
			t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)", want, len(want), got, len(got), string(want), string(got))
		}

		out = out[:0]

		got = Bytes([]byte(input), out)
		if !bytes.Equal(got, want) {
			t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)", want, len(want), got, len(got), string(want), string(got))
		}

		got = Bytes([]byte(input), out)
		if !bytes.Equal(got, want) {
			t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)", want, len(want), got, len(got), string(want), string(got))
		}
	})

	run := func(t *testing.T, input string, expectedOutput string) (string, func(t *testing.T)) {
		return fmt.Sprintf("%s should be %s", input, expectedOutput), func(t *testing.T) {
			out := make([]byte, len(input))
			result := Bytes([]byte(input), out)

			assert.Equal(t, expectedOutput, string(result))
		}
	}

	t.Run(run(t, `"foo"`, `\"foo\"`))
	t.Run(run(t, `foo\n`, `foo\n`))
	t.Run(run(t, `foo\t`, `foo\t`))
	t.Run(run(t, `{"test": "{\"foo\": \"bar\", \"re\":\"\\w+\"}"}`, `{\"test\": \"{\\\"foo\\\": \\\"bar\\\", \\\"re\\\":\\\"\\\\w+\\\"}\"}`))
	t.Run(run(t, `"Hello, 世界"`, `\"Hello, 世界\"`))

}

func BenchmarkBytes(b *testing.B) {
	input := `foo
	bar
  baz	bal
`
	inputBytes := []byte(input)
	out := make([]byte, len(inputBytes)*2)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if len(inputBytes) != len(input) {
			b.Fatalf("must be same len")
		}
		out = Bytes(inputBytes, out)
		if len(out) == 0 {
			b.Fatalf("must not be 0")
		}
	}
}
