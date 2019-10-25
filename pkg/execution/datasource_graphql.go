package execution

import (
	"bytes"
	"encoding/json"
	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type GraphQLDataSourcePlanner struct {
}

func (g *GraphQLDataSourcePlanner) DirectiveName() []byte {
	return []byte("GraphQLDataSource")
}

func (g *GraphQLDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (g *GraphQLDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (g *GraphQLDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (g *GraphQLDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (g *GraphQLDataSourcePlanner) EnterField(ref int) {

}

func (g *GraphQLDataSourcePlanner) LeaveField(ref int) {

}

func (g *GraphQLDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &GraphQLDataSource{}, nil
}

type GraphQLDataSource struct{}

func (g *GraphQLDataSource) Resolve(ctx Context, args ResolvedArgs) []byte {

	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)
	queryArg := args.ByKey(literal.QUERY)

	if hostArg == nil || urlArg == nil || queryArg == nil {
		log.Fatal("one of host,url,query arg nil")
		return nil
	}

	url := string(hostArg) + string(urlArg)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		url = "https://" + url
	}

	variables := map[string]json.RawMessage{}
	for i := 0; i < len(args); i++ {
		key := args[i].Key
		switch {
		case bytes.Equal(key, literal.HOST):
		case bytes.Equal(key, literal.URL):
		case bytes.Equal(key, literal.QUERY):
		default:
			variables[string(key)] = args[i].Value
		}
	}

	gqlRequest := GraphqlRequest{
		OperationName: "o",
		Variables:     variables,
		Query:         string(queryArg),
	}

	gqlRequestData, err := json.MarshalIndent(gqlRequest, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	client := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0 * time.Second,
		},
	}

	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(gqlRequestData))
	if err != nil {
		log.Fatal(err)
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")

	res, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	data = bytes.ReplaceAll(data, literal.BACKSLASH, nil)
	data, _, _, err = jsonparser.Get(data, "data")
	if err != nil {
		log.Fatal(err)
	}
	return data
}
