package graphql_http_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

type SubscriptionConfiguration struct {
	URL string
}

type FetchConfiguration struct {
	URL    string
	Method string
	Header http.Header
}

type HTTPConfiguration struct {
	Fetch        FetchConfiguration
	Subscription SubscriptionConfiguration
}

func (c *HTTPConfiguration) ApplyDefaults() {
	if c.Fetch.Method == "" {
		c.Fetch.Method = "POST"
	}
}

type HTTPSource struct {
	config             HTTPConfiguration
	fetchClient        *http.Client
	subscriptionClient graphql_datasource.GraphQLSubscriptionClient
}

func (p *HTTPSource) ApplyDefaults() {
	p.config.ApplyDefaults()
}

func (c *HTTPSource) SetConfig(config HTTPConfiguration) {
	c.config = config
}

func (p *HTTPSource) ConfigureFetch(params graphql_datasource.ConfigureFetchParams) graphql_datasource.ConfigureFetchResponse {
	var input []byte
	input = httpclient.SetInputBodyWithPath(input, params.UpstreamVariables, "variables")
	input = httpclient.SetInputBodyWithPath(input, params.Operation, "query")

	header, err := json.Marshal(p.config.Fetch.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	input = httpclient.SetInputURL(input, []byte(p.config.Fetch.URL))
	input = httpclient.SetInputMethod(input, []byte(p.config.Fetch.Method))

	return graphql_datasource.ConfigureFetchResponse{
		Input: string(input),
		DataSource: &FetchSource{
			httpClient: p.fetchClient,
		},
	}
}

type FetchSource struct {
	httpClient *http.Client
}

func (s *FetchSource) compactAndUnNullVariables(input []byte) []byte {
	variables, _, _, err := jsonparser.Get(input, "body", "variables")
	if err != nil {
		return input
	}
	if bytes.Equal(variables, []byte("null")) || bytes.Equal(variables, []byte("{}")) {
		return input
	}
	if bytes.ContainsAny(variables, " \t\n\r") {
		buf := bytes.NewBuffer(make([]byte, 0, len(variables)))
		_ = json.Compact(buf, variables)
		variables = buf.Bytes()
	}
	cp := make([]byte, len(variables))
	copy(cp, variables)
	variables = cp
	var changed bool
	for {
		variables, changed = s.unNullVariables(variables)
		if !changed {
			break
		}
	}
	input, _ = jsonparser.Set(input, variables, "body", "variables")
	return input
}

func (s *FetchSource) unNullVariables(input []byte) ([]byte, bool) {
	if i := bytes.Index(input, []byte(":{}")); i != -1 {
		end := i + 3
		hasTrainlingComma := false
		if input[end] == ',' {
			end++
			hasTrainlingComma = true
		}
		startQuote := bytes.LastIndex(input[:i-2], []byte("\""))
		if !hasTrainlingComma && input[startQuote-1] == ',' {
			startQuote--
		}
		return append(input[:startQuote], input[end:]...), true
	}
	if i := bytes.Index(input, []byte("null")); i != -1 {
		end := i + 4
		hasTrailingComma := false
		if input[end] == ',' {
			end++
			hasTrailingComma = true
		}
		startQuote := bytes.LastIndex(input[:i-2], []byte("\""))
		if !hasTrailingComma && input[startQuote-1] == ',' {
			startQuote--
		}
		return append(input[:startQuote], input[end:]...), true
	}
	return input, false
}

func (s *FetchSource) Load(ctx context.Context, input []byte, writer io.Writer) (err error) {
	input = s.compactAndUnNullVariables(input)
	return httpclient.Do(s.httpClient, ctx, input, writer)
}

func (p *HTTPSource) ConfigureSubscription(params graphql_datasource.ConfigureSubscriptionParams) graphql_datasource.ConfigureSubscriptionResponse {
	input := httpclient.SetInputBodyWithPath(nil, params.UpstreamVariables, "variables")
	input = httpclient.SetInputBodyWithPath(input, params.Operation, "query")
	input = httpclient.SetInputURL(input, []byte(p.config.Subscription.URL))

	header, err := json.Marshal(p.config.Fetch.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	return graphql_datasource.ConfigureSubscriptionResponse{
		Input: string(input),
		DataSource: &SubscriptionSource{
			client: p.subscriptionClient,
		},
	}
}

type SubscriptionSource struct {
	client graphql_datasource.GraphQLSubscriptionClient
}

func (s *SubscriptionSource) Start(ctx context.Context, input []byte, next chan<- []byte) error {
	var options graphql_datasource.GraphQLSubscriptionOptions
	err := json.Unmarshal(input, &options)
	if err != nil {
		return err
	}
	if options.Body.Query == "" {
		return resolve.ErrUnableToResolve
	}
	return s.client.Subscribe(ctx, options, next)
}

type Factory struct {
	BatchFactory resolve.DataSourceBatchFactory
	HTTPClient   *http.Client
	wsClient     *WebSocketGraphQLSubscriptionClient
}

func (f *Factory) Planner(ctx context.Context) plan.DataSourcePlanner {
	if f.wsClient == nil {
		f.wsClient = NewWebSocketGraphQLSubscriptionClient(f.HTTPClient, ctx)
	}

	source := &HTTPSource{
		fetchClient:        f.HTTPClient,
		subscriptionClient: f.wsClient,
	}

	return graphql_datasource.NewPlanner[HTTPConfiguration](
		f.BatchFactory,
		source,
	)
}
