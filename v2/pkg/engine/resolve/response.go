package resolve

import (
	"io"

	"github.com/buger/jsonparser"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

type GraphQLSubscription struct {
	Trigger  GraphQLSubscriptionTrigger
	Response *GraphQLResponse
}

type GraphQLSubscriptionTrigger struct {
	Input          []byte
	InputTemplate  InputTemplate
	Variables      Variables
	Source         SubscriptionDataSource
	PostProcessing PostProcessingConfiguration
}

type GraphQLResponse struct {
	Data            Node
	RenameTypeNames []RenameTypeName
}

type RenameTypeName struct {
	From, To []byte
}

type FlushWriter interface {
	io.Writer
	Flush()
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

func writeAndFlush(writer FlushWriter, msg []byte) error {
	_, err := writer.Write(msg)
	if err != nil {
		return err
	}
	writer.Flush()
	return nil
}

func extractResponse(responseData []byte, bufPair *BufPair, cfg PostProcessingConfiguration) {
	if len(responseData) == 0 {
		return
	}
	switch {
	case cfg.SelectResponseDataPath == nil && cfg.SelectResponseErrorsPath == nil:
		bufPair.Data.WriteBytes(responseData)
		return
	case cfg.SelectResponseDataPath != nil && cfg.SelectResponseErrorsPath == nil:
		data, _, _, _ := jsonparser.Get(responseData, cfg.SelectResponseDataPath...)
		bufPair.Data.WriteBytes(data)
		return
	case cfg.SelectResponseDataPath == nil && cfg.SelectResponseErrorsPath != nil:
		errors, _, _, _ := jsonparser.Get(responseData, cfg.SelectResponseErrorsPath...)
		bufPair.Errors.WriteBytes(errors)
	case cfg.SelectResponseDataPath != nil && cfg.SelectResponseErrorsPath != nil:
		jsonparser.EachKey(responseData, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
			switch i {
			case 0:
				bufPair.Data.WriteBytes(bytes)
			case 1:
				_, err := jsonparser.ArrayEach(bytes, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
					var (
						message, locations, path, extensions []byte
					)
					jsonparser.EachKey(value, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
						switch i {
						case errorsMessagePathIndex:
							message = bytes
						case errorsLocationsPathIndex:
							locations = bytes
						case errorsPathPathIndex:
							path = bytes
						case errorsExtensionsPathIndex:
							extensions = bytes
						}
					}, errorPaths...)
					bufPair.WriteErr(message, locations, path, extensions)
				})
				if err != nil {
					bufPair.WriteErr([]byte(err.Error()), nil, nil, nil)
				}
			}
		}, cfg.SelectResponseDataPath, cfg.SelectResponseErrorsPath)
	}
}
