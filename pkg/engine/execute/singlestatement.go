package execute

import (
	"bytes"
	"io"

	statement "github.com/jensneuse/graphql-go-tools/pkg/engine/statementv3"
)

func (e *Executor) PrepareSingleStatement(stmt statement.SingleStatement) (id int, err error) {
	e.singleStatements = append(e.singleStatements, stmt)
	return
}

func (e *Executor) ExecutePreparedSingleStatement(ctx Context, id int, out io.Writer) (n int, err error) {

	return
}

type storage struct {
	buffers []bytes.Buffer
}

func (e *Executor) resolveData() {

}

type PreparedSingleStatement struct {
	ResolveInstructions []ResolveInstruction
	Template            string
}

type ResolveInstruction struct {
	Resolver    statement.Resolver
	BufferIndex int
}
