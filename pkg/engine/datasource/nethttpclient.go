package datasource

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
)

type NetHttpClient struct {
	client *http.Client
}

func NewNetHttpClient(client *http.Client) *NetHttpClient {
	return &NetHttpClient{
		client: client,
	}
}

func (n *NetHttpClient) Do(ctx context.Context, url, method, headers, body []byte, out io.Writer) (err error) {

	var (
		bodyReader *bytes.Reader
	)

	if len(body) != 0 {
		bodyReader = bytes.NewReader(body)
	}

	request, err := http.NewRequestWithContext(ctx, unsafebytes.BytesToString(method), unsafebytes.BytesToString(url), bodyReader)
	if err != nil {
		return err
	}

	if len(headers) != 0 {
		err = jsonparser.ObjectEach(headers, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			request.Header.Add(unsafebytes.BytesToString(key), unsafebytes.BytesToString(value))
			return nil
		})
		if err != nil {
			return err
		}
	}

	request.Header.Add("Accept", "application/json")

	response, err := n.client.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	_, err = io.Copy(out, response.Body)
	return
}
