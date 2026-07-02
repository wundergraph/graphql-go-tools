package planbytecode

type ByteRange struct {
	Start int
	End   int
}

type ValueRangeStatus uint8

const (
	ValueRangeNotFound ValueRangeStatus = iota
	ValueRangeFound
	ValueRangeUnsupported
)

func FindValueRange(src []byte, path []string) (ByteRange, bool) {
	r, status := FindValueRangeStatus(src, path)
	return r, status == ValueRangeFound
}

func FindValueRangeStatus(src []byte, path []string) (ByteRange, ValueRangeStatus) {
	i := skipWS(src, 0)

	if len(path) == 0 {
		end, ok := skipJSONValue(src, i)

		if !ok {
			return ByteRange{}, ValueRangeUnsupported
		}

		return ByteRange{Start: i, End: end}, ValueRangeFound
	}

	for depth, key := range path {
		valueStart, valueEnd, status := findObjectFieldValueRange(src, i, key)

		if status != ValueRangeFound {
			return ByteRange{}, status
		}

		if depth == len(path)-1 {
			return ByteRange{Start: valueStart, End: valueEnd}, ValueRangeFound
		}

		i = skipWS(src, valueStart)
	}

	return ByteRange{}, ValueRangeNotFound
}

func ValueRangeIsNull(src []byte, r ByteRange) bool {
	i := skipWS(src, r.Start)
	return i+4 <= r.End && src[i] == 'n' && src[i+1] == 'u' && src[i+2] == 'l' && src[i+3] == 'l'
}

func ValueRangeIsEmptyArray(src []byte, r ByteRange) bool {
	i := skipWS(src, r.Start)

	if i >= r.End || src[i] != '[' {
		return false
	}

	i = skipWS(src, i+1)

	return i < r.End && src[i] == ']'
}

func ScanArrayValueRanges(
	src []byte, arrayRange ByteRange, ranges []ByteRange,
) ([]ByteRange, ValueRangeStatus) {
	i := skipWS(src, arrayRange.Start)

	if i >= arrayRange.End || src[i] != '[' {
		return ranges, ValueRangeNotFound
	}

	i++

	for {
		i = skipWS(src, i)

		if i >= arrayRange.End {
			return ranges, ValueRangeUnsupported
		}

		if src[i] == ']' {
			return ranges, ValueRangeFound
		}

		valueStart := i
		valueEnd, ok := skipJSONValue(src, valueStart)

		if !ok || valueEnd > arrayRange.End {
			return ranges, ValueRangeUnsupported
		}

		ranges = append(ranges, ByteRange{Start: valueStart, End: valueEnd})

		i = skipWS(src, valueEnd)

		if i >= arrayRange.End {
			return ranges, ValueRangeUnsupported
		}

		switch src[i] {
		case ',':
			i++
		case ']':
			return ranges, ValueRangeFound
		default:
			return ranges, ValueRangeUnsupported
		}
	}
}

func ScanObjectFieldRanges(
	src []byte, orderedFields []string, ranges []ByteRange,
) ([]ByteRange, bool) {
	i := skipWS(src, 0)

	if i >= len(src) || src[i] != '{' {
		return ranges, false
	}

	i++

	fieldIdx := 0

	for {
		i = skipWS(src, i)

		if i >= len(src) {
			return ranges, false
		}

		if src[i] == '}' {
			return ranges, fieldIdx == len(orderedFields)
		}

		if src[i] != '"' {
			return ranges, false
		}

		pairStart := i
		keyStart := i + 1
		keyEnd, escaped, ok := skipJSONString(src, i)

		if !ok || escaped {
			return ranges, false
		}

		i = skipWS(src, keyEnd+1)

		if i >= len(src) || src[i] != ':' {
			return ranges, false
		}

		i = skipWS(src, i+1)
		valueEnd, ok := skipJSONValue(src, i)

		if !ok {
			return ranges, false
		}

		if fieldIdx < len(orderedFields) && bytesEqualString(
			src[keyStart:keyEnd], orderedFields[fieldIdx],
		) {
			ranges = append(ranges, ByteRange{Start: pairStart, End: valueEnd})
			fieldIdx++
		} else if nextFieldIndex(src[keyStart:keyEnd], orderedFields[fieldIdx:]) > 0 {
			return ranges, false
		}

		i = skipWS(src, valueEnd)

		if i >= len(src) {
			return ranges, false
		}

		switch src[i] {
		case ',':
			i++
		case '}':
			return ranges, fieldIdx == len(orderedFields)
		default:
			return ranges, false
		}
	}
}

func AppendObjectFromRanges(dst []byte, src []byte, ranges []ByteRange) []byte {
	dst = append(dst, '{')

	for i, r := range ranges {
		if i != 0 {
			dst = append(dst, ',')
		}

		dst = append(dst, src[r.Start:r.End]...)
	}

	dst = append(dst, '}')
	return dst
}

func findObjectFieldValueRange(
	src []byte, i int, field string,
) (start int, end int, status ValueRangeStatus) {
	i = skipWS(src, i)

	if i >= len(src) || src[i] != '{' {
		return 0, 0, ValueRangeNotFound
	}

	i++

	for {
		i = skipWS(src, i)

		if i >= len(src) {
			return 0, 0, ValueRangeUnsupported
		}

		if src[i] == '}' {
			return 0, 0, ValueRangeNotFound
		}

		if src[i] != '"' {
			return 0, 0, ValueRangeUnsupported
		}

		keyStart := i + 1
		keyEnd, escaped, ok := skipJSONString(src, i)

		if !ok || escaped {
			return 0, 0, ValueRangeUnsupported
		}

		i = skipWS(src, keyEnd+1)

		if i >= len(src) || src[i] != ':' {
			return 0, 0, ValueRangeUnsupported
		}

		valueStart := skipWS(src, i+1)
		valueEnd, ok := skipJSONValue(src, valueStart)

		if !ok {
			return 0, 0, ValueRangeUnsupported
		}

		if bytesEqualString(src[keyStart:keyEnd], field) {
			return valueStart, valueEnd, ValueRangeFound
		}

		i = skipWS(src, valueEnd)

		if i >= len(src) {
			return 0, 0, ValueRangeUnsupported
		}

		switch src[i] {
		case ',':
			i++
		case '}':
			return 0, 0, ValueRangeNotFound
		default:
			return 0, 0, ValueRangeUnsupported
		}
	}
}

func nextFieldIndex(key []byte, fields []string) int {
	for i, field := range fields {
		if bytesEqualString(key, field) {
			return i
		}
	}

	return -1
}

func bytesEqualString(left []byte, right string) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}

func skipWS(src []byte, i int) int {
	for i < len(src) {
		switch src[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}

	return i
}

func skipJSONValue(src []byte, i int) (int, bool) {
	if i >= len(src) {
		return i, false
	}

	switch src[i] {
	case '"':
		end, _, ok := skipJSONString(src, i)
		if !ok {
			return i, false
		}
		return end + 1, true
	case '{':
		return skipJSONComposite(src, i, '{', '}')
	case '[':
		return skipJSONComposite(src, i, '[', ']')
	default:
		return skipJSONScalar(src, i)
	}
}

func skipJSONString(src []byte, i int) (end int, escaped bool, ok bool) {
	if i >= len(src) || src[i] != '"' {
		return i, false, false
	}

	i++

	for i < len(src) {
		switch src[i] {
		case '\\':
			escaped = true
			i += 2
		case '"':
			return i, escaped, true
		default:
			i++
		}
	}

	return i, escaped, false
}

func skipJSONComposite(src []byte, i int, open, close byte) (int, bool) {
	if i >= len(src) || src[i] != open {
		return i, false
	}

	depth := 1
	i++

	for i < len(src) {
		switch src[i] {
		case '"':
			end, _, ok := skipJSONString(src, i)

			if !ok {
				return i, false
			}

			i = end + 1
		case open:
			depth++
			i++
		case close:
			depth--
			i++

			if depth == 0 {
				return i, true
			}
		default:
			i++
		}
	}

	return i, false
}

func skipJSONScalar(src []byte, i int) (int, bool) {
	start := i

	for i < len(src) {
		switch src[i] {
		case ',', '}', ']', ' ', '\n', '\r', '\t':
			return i, i > start
		default:
			i++
		}
	}

	return i, i > start
}
