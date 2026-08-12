package ast

// indexOf - simple helper to find an index of a ref within refs slice
func indexOf(refs []int, ref int) (int, bool) {
	for i, j := range refs {
		if ref == j {
			return i, true
		}
	}
	return -1, false
}

// deleteRef - is a slice trick to remove an item with preserving items order
// Note: danger modifies pointer to the arr
func deleteRef(refs *[]int, index int) {
	*refs = append((*refs)[:index], (*refs)[index+1:]...)
}

// Splits byte slices into lines based on line terminators (\n, \r, \r\n)
// defined by https://spec.graphql.org/October2021/#sec-Line-Terminators
func splitBytesIntoLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	length := len(data)

	for i := 0; i < length; i++ {
		switch c := data[i]; c {
		case '\n', '\r':
			if start <= i {
				lines = append(lines, data[start:i])
			}

			if c == '\r' && i+1 < length && data[i+1] == '\n' {
				i++
			}

			start = i + 1
		}
	}

	if start <= length {
		lines = append(lines, data[start:])
	}

	return lines
}

// counts leading whitespace characters (spaces or tabs) in a byte slice
func leadingWhitespaceCount(line []byte) int {
	count := 0
	for _, c := range line {
		if c != ' ' && c != '\t' {
			break
		}
		count++
	}
	return count
}

// commonBlockStringIndent returns the common leading-whitespace length of
// every non-empty line after the first, per step 3 of the GraphQL spec's
// BlockStringValue() canonicalization. Lines that are all whitespace are
// excluded. Returns -1 when there is nothing to strip.
func commonBlockStringIndent(lines [][]byte) int {
	common := -1
	for i, line := range lines {
		if i == 0 {
			continue
		}
		indent := leadingWhitespaceCount(line)
		if indent < len(line) {
			if common == -1 || indent < common {
				common = indent
			}
		}
	}
	return common
}
