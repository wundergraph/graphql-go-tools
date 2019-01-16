package position

import "fmt"

type Position struct {
	LineStart int
	LineEnd   int
	CharStart int
	CharEnd   int
}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d-%d:%d", p.LineStart, p.CharStart, p.LineEnd, p.CharEnd)
}

func (p *Position) SetEnd(position Position) {
	p.LineEnd = position.LineEnd
	p.CharEnd = position.CharEnd
}
