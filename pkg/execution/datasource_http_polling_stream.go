package execution

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"
)

type HttpPollingStreamDataSourcePlanner struct {
	BaseDataSourcePlanner
	rootField int
	delay     time.Duration
}

func (h *HttpPollingStreamDataSourcePlanner) OverrideRootFieldPath(path []string) []string {
	return nil
}

func NewHttpPollingStreamDataSourcePlanner(log *zap.Logger) *HttpPollingStreamDataSourcePlanner {
	return &HttpPollingStreamDataSourcePlanner{
		BaseDataSourcePlanner: BaseDataSourcePlanner{
			log: log,
		},
	}
}

func (h *HttpPollingStreamDataSourcePlanner) DirectiveName() []byte {
	return []byte("HttpPollingStreamDataSource")
}

func (h *HttpPollingStreamDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &HttpPollingStreamDataSource{
		log:   h.log,
		delay: h.delay,
	}, h.args
}

func (h *HttpPollingStreamDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	h.walker, h.operation, h.definition, h.args = walker, operation, definition, args
	h.rootField = -1
	h.delay = time.Second * 1
}

func (h *HttpPollingStreamDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) EnterField(ref int) {
	if h.rootField == -1 {
		h.rootField = ref
	}
}

func (h *HttpPollingStreamDataSourcePlanner) LeaveField(ref int) {
	if h.rootField != ref {
		return
	}

	definition, exists := h.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	directive, exists := h.definition.FieldDefinitionDirectiveByName(definition, h.DirectiveName())
	if !exists {
		return
	}
	value, exists := h.definition.DirectiveArgumentValueByName(directive, literal.URL)
	if !exists {
		return
	}
	variableValue := h.definition.StringValueContentBytes(value.Ref)
	arg := &StaticVariableArgument{
		Name:  literal.URL,
		Value: variableValue,
	}
	h.args = append([]Argument{arg}, h.args...)
	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.HOST)
	if !exists {
		return
	}
	variableValue = h.definition.StringValueContentBytes(value.Ref)
	arg = &StaticVariableArgument{
		Name:  literal.HOST,
		Value: variableValue,
	}
	h.args = append([]Argument{arg}, h.args...)
	h.setDelayFromDirective(ref, directive)
}

func (h *HttpPollingStreamDataSourcePlanner) setDelayFromDirective(field, directive int) {
	value, exists := h.definition.DirectiveArgumentValueByName(directive, []byte("delaySeconds"))
	if !exists || value.Kind != ast.ValueKindInteger {
		h.setDefaultDelay()
		return
	}
	delaySeconds := h.definition.IntValueAsInt(value.Ref)
	h.delay = time.Second * time.Duration(delaySeconds)
}

func (h *HttpPollingStreamDataSourcePlanner) setDefaultDelay() {
	inputValueDefinition := h.definition.DirectiveArgumentInputValueDefinition([]byte("HttpPollingStreamDataSource"), []byte("delaySeconds"))
	if inputValueDefinition == -1 {
		return
	}
	if !h.definition.InputValueDefinitionHasDefaultValue(inputValueDefinition) {
		return
	}
	value := h.definition.InputValueDefinitionDefaultValue(inputValueDefinition)
	if value.Kind != ast.ValueKindInteger {
		return
	}
	delaySeconds := h.definition.IntValueAsInt(value.Ref)
	h.delay = time.Second * time.Duration(delaySeconds)
}

type HttpPollingStreamDataSource struct {
	log      *zap.Logger
	once     sync.Once
	ch       chan []byte
	closed   bool
	delay    time.Duration
	client   *http.Client
	request  *http.Request
	lastData []byte
}

func (h *HttpPollingStreamDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	h.once.Do(func() {
		h.ch = make(chan []byte)
		h.request = h.generateRequest(args)
		h.client = &http.Client{
			Timeout: time.Second * 5,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
				TLSHandshakeTimeout: 0 * time.Second,
			},
		}
		go h.startPolling(ctx)
	})
	if h.closed {
		return CloseConnection
	}
	select {
	case data := <-h.ch:
		h.log.Debug("HttpPollingStreamDataSource.Resolve.out.Write",
			zap.ByteString("data", data),
		)
		_, err := out.Write(data)
		if err != nil {
			h.log.Error("HttpPollingStreamDataSource.Resolve",
				zap.Error(err),
			)
		}
	case <-ctx.Done():
		h.closed = true
		return CloseConnection
	}
	return KeepStreamAlive
}

func (h *HttpPollingStreamDataSource) startPolling(ctx Context) {
	first := true
	for {
		if first {
			first = !first
		} else {
			time.Sleep(h.delay)
		}
		var data []byte
		select {
		case <-ctx.Done():
			h.closed = true
			return
		default:
			response, err := h.client.Do(h.request)
			if err != nil {
				h.log.Error("HttpPollingStreamDataSource.startPolling.client.Do",
					zap.Error(err),
				)
				return
			}
			data, err = ioutil.ReadAll(response.Body)
			if err != nil {
				h.log.Error("HttpPollingStreamDataSource.startPolling.ioutil.ReadAll",
					zap.Error(err),
				)
				return
			}
		}
		if bytes.Equal(data, h.lastData) {
			continue
		}
		h.lastData = data
		select {
		case <-ctx.Done():
			h.closed = true
			return
		case h.ch <- data:
			continue
		}
	}
}

func (h *HttpPollingStreamDataSource) generateRequest(args ResolvedArgs) *http.Request {
	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)

	h.log.Debug("HttpPollingStreamDataSource.generateRequest.Resolve.args",
		zap.Strings("resolvedArgs", args.Dump()),
	)

	if hostArg == nil || urlArg == nil {
		h.log.Error("HttpPollingStreamDataSource.generateRequest.args invalid")
		return nil
	}

	url := string(hostArg) + string(urlArg)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		url = "https://" + url
	}

	if strings.Contains(url, "{{") {
		tmpl, err := template.New("url").Parse(url)
		if err != nil {
			h.log.Error("HttpPollingStreamDataSource.generateRequest.template.New",
				zap.Error(err),
			)
			return nil
		}
		out := bytes.Buffer{}
		data := make(map[string]string, len(args))
		for i := 0; i < len(args); i++ {
			data[string(args[i].Key)] = string(args[i].Value)
		}
		err = tmpl.Execute(&out, data)
		if err != nil {
			h.log.Error("HttpPollingStreamDataSource.generateRequest.tmpl.Execute",
				zap.Error(err),
			)
			return nil
		}
		url = out.String()
	}

	h.log.Debug("HttpPollingStreamDataSource.generateRequest.Resolve",
		zap.String("url", url),
	)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		h.log.Error("HttpPollingStreamDataSource.generateRequest.Resolve.NewRequest",
			zap.Error(err),
		)
		return nil
	}
	request.Header.Add("Accept", "application/json")
	return request
}
