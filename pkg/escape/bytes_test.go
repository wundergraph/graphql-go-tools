package escape

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestBytes(t *testing.T) {
	input := `foo
	bar
  baz	bal
"str"
`

	marshalled,err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	want := marshalled[1:len(marshalled)-1]

	var out []byte

	got := Bytes([]byte(input),out)
	if !bytes.Equal(got,want){
		t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)",want,len(want),got,len(got),string(want),string(got))
	}

	out = make([]byte,len(input))

	got = Bytes([]byte(input),out)
	if !bytes.Equal(got,want){
		t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)",want,len(want),got,len(got),string(want),string(got))
	}

	out = out[:0]

	got = Bytes([]byte(input),out)
	if !bytes.Equal(got,want){
		t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)",want,len(want),got,len(got),string(want),string(got))
	}

	got = Bytes([]byte(input),out)
	if !bytes.Equal(got,want){
		t.Fatalf("\n%+v (want: %d)\n%+v (got: %d)\n%s (wantString)\n%s (gotString)",want,len(want),got,len(got),string(want),string(got))
	}
}

func BenchmarkBytes(b *testing.B) {
	input := `foo
	bar
  baz	bal
`
	inputBytes := []byte(input)
	out := make([]byte,len(inputBytes) * 2)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0;i<b.N;i++{
		if len(inputBytes) != len(input){
			b.Fatalf("must be same len")
		}
		out = Bytes(inputBytes,out)
		if len(out) == 0 {
			b.Fatalf("must not be 0")
		}
	}
}