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

func (p *Position) MergeStartIntoStart(position Position) {
	p.LineStart = position.LineStart
	p.CharStart = position.CharStart
}

func (p *Position) MergeStartIntoEnd(position Position) {
	p.LineEnd = position.LineStart
	p.CharEnd = position.CharStart
}

func (p *Position) MergeEndIntoEnd(position Position) {
	p.LineEnd = position.LineEnd
	p.CharEnd = position.CharEnd
}
