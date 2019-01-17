package position

import "fmt"

type Position struct {
	LineStart uint16
	LineEnd   uint16
	CharStart uint16
	CharEnd   uint16
}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d-%d:%d", p.LineStart, p.CharStart, p.LineEnd, p.CharEnd)
}
