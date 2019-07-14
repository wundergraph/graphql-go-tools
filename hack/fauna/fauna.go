package fauna

import (
	"bytes"
	"encoding/json"
	"fmt"
	f "github.com/fauna/faunadb-go/faunadb"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
)

type LoggingRoundTripper struct {
	httpRoundTripper http.RoundTripper
}

func NewLoggingRoundTripper() *LoggingRoundTripper {
	return &LoggingRoundTripper{
		httpRoundTripper: http.DefaultTransport,
	}
}

func (l *LoggingRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {

	err = l.printBody(req)
	if err != nil {
		return nil, err
	}

	return l.httpRoundTripper.RoundTrip(req)
}

func (l *LoggingRoundTripper) printBody(req *http.Request) error {

	buff := bytes.Buffer{}

	_, err := io.Copy(&buff, req.Body)
	if err != nil {
		return err
	}

	intermediate := map[string]interface{}{}
	err = json.Unmarshal(buff.Bytes(), &intermediate)
	if err != nil {
		return nil
	}

	pretty, err := json.MarshalIndent(intermediate, "", "  ")
	if err != nil {
		return err
	}

	req.Body = ioutil.NopCloser(bytes.NewReader(buff.Bytes()))

	fmt.Printf("Sending Body:\n\n%s\n\n", string(pretty))
	return nil
}

func prettyPrint(value f.Value) (string, error) {
	uglified, err := f.MarshalJSON(value)
	if err != nil {
		return "", err
	}
	unmarshalled := map[string]interface{}{}
	err = json.Unmarshal(uglified, &unmarshalled)
	prettified, err := json.MarshalIndent(unmarshalled, "", "  ")
	if err != nil {
		return "", err
	}
	return string(prettified), nil
}

func run(t *testing.T, client *f.FaunaClient, expr f.Expr) {
	result, err := client.Query(expr)
	if err != nil {
		t.Fatal(err)
	}

	out, err := prettyPrint(result)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Response:\n\n%s", out)
}
