package resolve

import (
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

type GraphQLSubscription struct {
	Trigger  GraphQLSubscriptionTrigger
	Response *GraphQLResponse
	Filter   *SubscriptionFilter
}

type GraphQLSubscriptionTrigger struct {
	Input          []byte
	InputTemplate  InputTemplate
	Variables      Variables
	Source         SubscriptionDataSource
	PostProcessing PostProcessingConfiguration
}

type GraphQLResponse struct {
	Data            *Object
	RenameTypeNames []RenameTypeName
	Info            *GraphQLResponseInfo
	Fetches         *FetchTreeNode
	DataSources     []DataSourceInfo
}

type GraphQLResponseInfo struct {
	OperationType ast.OperationType
}

type RenameTypeName struct {
	From, To []byte
}

type ResponseWriter interface {
	io.Writer
}

type SubscriptionResponseWriter interface {
	ResponseWriter
	Flush() error
	Complete()
}

func writeGraphqlResponse(buf *BufPair, writer io.Writer, ignoreData bool) (err error) {
	hasErrors := buf.Errors.Len() != 0
	hasData := buf.Data.Len() != 0 && !ignoreData

	err = writeSafe(err, writer, lBrace)

	if hasErrors {
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literalErrors)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		err = writeSafe(err, writer, lBrack)
		err = writeSafe(err, writer, buf.Errors.Bytes())
		err = writeSafe(err, writer, rBrack)
		err = writeSafe(err, writer, comma)
	}

	err = writeSafe(err, writer, quote)
	err = writeSafe(err, writer, literalData)
	err = writeSafe(err, writer, quote)
	err = writeSafe(err, writer, colon)

	if hasData {
		_, err = writer.Write(buf.Data.Bytes())
	} else {
		err = writeSafe(err, writer, literal.NULL)
	}
	err = writeSafe(err, writer, rBrace)

	return err
}

func writeSafe(err error, writer io.Writer, data []byte) error {
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func writeFlushComplete(writer SubscriptionResponseWriter, msg []byte) error {
	_, err := writer.Write(msg)
	if err != nil {
		return err
	}
	err = writer.Flush()
	if err != nil {
		return err
	}
	writer.Complete()
	return nil
}
