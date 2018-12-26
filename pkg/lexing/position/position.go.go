package position

import "fmt"

type Position struct {
	Line int
	Char int
}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Char)
}
