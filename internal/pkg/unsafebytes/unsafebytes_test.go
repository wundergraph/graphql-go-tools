package unsafebytes

import "testing"

func TestByteSliceToInt(t *testing.T) {
	got := BytesToInt64([]byte("10"))
	if got != 10 {
		t.Fatalf("want 10, got: %d", got)
	}
	got = BytesToInt64([]byte("01"))
	if got != 1 {
		t.Fatalf("want 1, got: %d", got)
	}
	got = BytesToInt64([]byte("0"))
	if got != 0 {
		t.Fatalf("want 0, got: %d", got)
	}
	got = BytesToInt64([]byte("-10"))
	if got != -10 {
		t.Fatalf("want -10, got: %d", got)
	}
}

func TestByteSliceToFloat(t *testing.T) {
	got := BytesToFloat32([]byte("10.24"))
	if got != 10.24 {
		t.Fatalf("want 10.24, got: %f", got)
	}
	got = BytesToFloat32([]byte("0"))
	if got != 0 {
		t.Fatalf("want 0, got: %f", got)
	}
	got = BytesToFloat32([]byte("001"))
	if got != 1 {
		t.Fatalf("want 1, got: %f", got)
	}
}

func TestBytesToString(t *testing.T) {
	got := BytesToString([]byte("foo"))
	if got != "foo" {
		t.Fatalf("want foo, got: %s", got)
	}
}

func BenchmarkByteSliceToInt(b *testing.B) {

	in := []byte("1024")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		out := BytesToInt64(in)
		if out != 1024 {
			b.Fatalf("want 1024, got: %d", out)
		}
	}
}

func BenchmarkByteSliceToFloat(b *testing.B) {

	in := []byte("10.24")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		out := BytesToFloat32(in)
		if out != 10.24 {
			b.Fatalf("want 1024, got: %f", out)
		}
	}
}

func BenchmarkByteSliceToString(b *testing.B) {

	in := []byte("foo")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		out := BytesToString(in)
		if out != "foo" {
			b.Fatalf("want 1024, got: %s", out)
		}
	}
}
