package execution

import (
	"bytes"
	"encoding/json"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"
)

type HttpPollingStreamDataSourceConfiguration struct {
	Host         string
	URL          string
	DelaySeconds *int
}

type HttpPollingStreamDataSourcePlannerFactoryFactory struct {
}

func (h HttpPollingStreamDataSourcePlannerFactoryFactory) Initialize(base BaseDataSourcePlanner, configReader io.Reader) (DataSourcePlannerFactory, error) {
	factory := &HttpPollingStreamDataSourcePlannerFactory{
		base: base,
	}
	return factory, json.NewDecoder(configReader).Decode(&factory.config)
}

type HttpPollingStreamDataSourcePlannerFactory struct {
	base   BaseDataSourcePlanner
	config HttpPollingStreamDataSourceConfiguration
}

func (h HttpPollingStreamDataSourcePlannerFactory) DataSourcePlanner() DataSourcePlanner {
	return &HttpPollingStreamDataSourcePlanner{
		BaseDataSourcePlanner: h.base,
		dataSourceConfig:      h.config,
	}
}

type HttpPollingStreamDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig HttpPollingStreamDataSourceConfiguration
	delay            time.Duration
}

func (h *HttpPollingStreamDataSourcePlanner) Plan(args []Argument) (DataSource, []Argument) {
	return &HttpPollingStreamDataSource{
		log:   h.log,
		delay: h.delay,
	}, append(h.args, args...)
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
	h.rootField.setIfNotDefined(ref)
}

func (h *HttpPollingStreamDataSourcePlanner) LeaveField(ref int) {
	if !h.rootField.isDefinedAndEquals(ref) {
		return
	}
	h.args = append(h.args, &StaticVariableArgument{
		Name:  literal.HOST,
		Value: []byte(h.dataSourceConfig.Host),
	})
	h.args = append(h.args, &StaticVariableArgument{
		Name:  literal.URL,
		Value: []byte(h.dataSourceConfig.URL),
	})
	if h.dataSourceConfig.DelaySeconds == nil {
		h.delay = time.Second * time.Duration(1)
	} else {
		h.delay = time.Second * time.Duration(*h.dataSourceConfig.DelaySeconds)
	}
}

type HttpPollingStreamDataSource struct {
	log      log.Logger
	once     sync.Once
	ch       chan []byte
	closed   bool
	delay    time.Duration
	client   *http.Client
	request  *http.Request
	lastData []byte
}

func (h *HttpPollingStreamDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) (n int, err error) {
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
		return
	}
	select {
	case data := <-h.ch:
		h.log.Debug("HttpPollingStreamDataSource.Resolve.out.Write",
			log.ByteString("data", data),
		)
		_, err := out.Write(data)
		if err != nil {
			h.log.Error("HttpPollingStreamDataSource.Resolve",
				log.Error(err),
			)
		}
	case <-ctx.Done():
		h.closed = true
		return
	}
	return
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
					log.Error(err),
				)
				return
			}
			data, err = ioutil.ReadAll(response.Body)
			if err != nil {
				h.log.Error("HttpPollingStreamDataSource.startPolling.ioutil.ReadAll",
					log.Error(err),
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
		log.Strings("resolvedArgs", args.Dump()),
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
				log.Error(err),
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
				log.Error(err),
			)
			return nil
		}
		url = out.String()
	}

	h.log.Debug("HttpPollingStreamDataSource.generateRequest.Resolve",
		log.String("url", url),
	)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		h.log.Error("HttpPollingStreamDataSource.generateRequest.Resolve.NewRequest",
			log.Error(err),
		)
		return nil
	}
	request.Header.Add("Accept", "application/json")
	return request
}
