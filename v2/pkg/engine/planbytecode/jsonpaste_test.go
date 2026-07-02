package planbytecode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanObjectFieldRangesCopiesRequestedFieldsInSchemaOrder(t *testing.T) {
	src := []byte(`{"id":"1","name":"Ada","reviews":[{"body":"ok","meta":{"score":10}}],"extra":true}`)
	ranges := make([]ByteRange, 0, 2)

	ranges, ok := ScanObjectFieldRanges(src, []string{"id", "reviews"}, ranges)
	require.True(t, ok)
	require.Equal(t, []ByteRange{
		{Start: 1, End: 9},
		{Start: 23, End: 68},
	}, ranges)

	out := AppendObjectFromRanges(make([]byte, 0, 64), src, ranges)
	require.JSONEq(t, `{"id":"1","reviews":[{"body":"ok","meta":{"score":10}}]}`, string(out))
}

func TestScanObjectFieldRangesRejectsOutOfOrderFields(t *testing.T) {
	src := []byte(`{"name":"Ada","id":"1"}`)
	ranges := make([]ByteRange, 0, 2)

	_, ok := ScanObjectFieldRanges(src, []string{"id", "name"}, ranges)
	require.False(t, ok)
}

func TestScanObjectFieldRangesIsAllocationFreeWithCallerOwnedScratch(t *testing.T) {
	src := []byte(`{"id":"1","name":"Ada","reviews":[{"body":"ok"}]}`)
	fields := []string{"id", "name", "reviews"}
	scratch := make([]ByteRange, 0, len(fields))

	allocs := testing.AllocsPerRun(1000, func() {
		ranges, ok := ScanObjectFieldRanges(src, fields, scratch[:0])
		if !ok || len(ranges) != len(fields) {
			t.Fatalf("unexpected scan result: ranges=%v ok=%v", ranges, ok)
		}
	})
	require.Zero(t, allocs)
}

func TestFindValueRangeFindsNestedObjectField(t *testing.T) {
	src := []byte(`{"errors":[],"data":{"_entities":[{"name":"Ada"}],"other":true}}`)

	r, ok := FindValueRange(src, []string{"data", "_entities"})
	require.True(t, ok)
	require.JSONEq(t, `[{"name":"Ada"}]`, string(src[r.Start:r.End]))

	r, ok = FindValueRange(src, []string{"errors"})
	require.True(t, ok)
	require.True(t, ValueRangeIsEmptyArray(src, r))
}

func TestFindValueRangeRejectsEscapedObjectKeys(t *testing.T) {
	src := []byte(`{"da\u0074a":{"id":"1"}}`)

	_, ok := FindValueRange(src, []string{"data"})
	require.False(t, ok)
	_, status := FindValueRangeStatus(src, []string{"data"})
	require.Equal(t, ValueRangeUnsupported, status)
}

func TestValueRangeIsNull(t *testing.T) {
	src := []byte(`{"data": null}`)

	r, ok := FindValueRange(src, []string{"data"})
	require.True(t, ok)
	require.True(t, ValueRangeIsNull(src, r))
}

func TestScanArrayValueRanges(t *testing.T) {
	src := []byte(`{"items":[{"id":1}, null, "x"]}`)
	r, ok := FindValueRange(src, []string{"items"})
	require.True(t, ok)

	ranges, status := ScanArrayValueRanges(src, r, nil)
	require.Equal(t, ValueRangeFound, status)
	require.Equal(t, []string{`{"id":1}`, `null`, `"x"`}, []string{
		string(src[ranges[0].Start:ranges[0].End]),
		string(src[ranges[1].Start:ranges[1].End]),
		string(src[ranges[2].Start:ranges[2].End]),
	})
}
