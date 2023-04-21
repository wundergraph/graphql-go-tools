package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	graphqlDataSource "github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
)

type ServiceConfig struct {
	Name string
	URL  string
	WS   string
	Fallback func(*ServiceConfig) (string, error)
}

type DatasourcePollerConfig struct {
	Services        []ServiceConfig
	PollingInterval time.Duration
}

const ServiceDefinitionQuery = `
	{
		"query": "query __ApolloGetServiceDefinition__ { _service { sdl } }",
		"operationName": "__ApolloGetServiceDefinition__",
		"variables": {}
	}`

type GQLErr []struct {
	Message string `json:"message"`
}

func (g GQLErr) Error() string {
	var builder strings.Builder
	for _, m := range g {
		_ = builder.WriteByte('\t')
		_, _ = builder.WriteString(m.Message)
	}

	return builder.String()
}

func NewDatasourcePoller(
	httpClient *http.Client,
	config DatasourcePollerConfig,
) *DatasourcePollerPoller {
	return &DatasourcePollerPoller{
		httpClient: httpClient,
		config:     config,
		sdlMap:     make(map[string]string),
	}
}

type DatasourcePollerPoller struct {
	httpClient *http.Client

	config DatasourcePollerConfig
	sdlMap map[string]string

	updateDatasourceObservers []DataSourceObserver
}

func (d *DatasourcePollerPoller) Register(updateDatasourceObserver DataSourceObserver) {
	d.updateDatasourceObservers = append(d.updateDatasourceObservers, updateDatasourceObserver)
}

func (d *DatasourcePollerPoller) Run(ctx context.Context) {
	d.updateSDLs(ctx)

	if d.config.PollingInterval == 0 {
		<-ctx.Done()
		return
	}

	ticker := time.NewTicker(d.config.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.updateSDLs(ctx)
		}
	}
}

func (d *DatasourcePollerPoller) updateSDLs(ctx context.Context) {
	d.sdlMap = make(map[string]string)

	var wg sync.WaitGroup
	resultCh := make(chan struct {
		name string
		sdl  string
	})

	for _, serviceConf := range d.config.Services {
		serviceConf := serviceConf // Create new instance of serviceConf for the goroutine.
		wg.Add(1)
		go func() {
			defer wg.Done()

			sdl, err := d.fetchServiceSDL(ctx, serviceConf.URL)
			if err != nil {
				log.Println("Failed to get sdl.", err)

				if serviceConf.Fallback == nil {
					return
				} else {
					sdl, err = serviceConf.Fallback(&serviceConf)
					if err != nil {
						log.Println("Failed to get sdl with fallback.", err)
						return
					}
				}
			}

			select {
			case <-ctx.Done():
			case resultCh <- struct {
				name string
				sdl  string
			}{name: serviceConf.Name, sdl: sdl}:
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for result := range resultCh {
		d.sdlMap[result.name] = result.sdl
	}

	d.updateObservers()
}

func (d *DatasourcePollerPoller) updateObservers() {
	dataSourceConfigs := d.createDatasourceConfig()

	for i := range d.updateDatasourceObservers {
		d.updateDatasourceObservers[i].UpdateDataSources(dataSourceConfigs)
	}
}

func (d *DatasourcePollerPoller) createDatasourceConfig() []graphqlDataSource.Configuration {
	dataSourceConfigs := make([]graphqlDataSource.Configuration, 0, len(d.config.Services))

	for _, serviceConfig := range d.config.Services {
		sdl, exists := d.sdlMap[serviceConfig.Name]
		if !exists {
			continue
		}

		dataSourceConfig := graphqlDataSource.Configuration{
			Fetch: graphqlDataSource.FetchConfiguration{
				URL:    serviceConfig.URL,
				Method: http.MethodPost,
			},
			Subscription: graphqlDataSource.SubscriptionConfiguration{
				URL: serviceConfig.WS,
			},
			Federation: graphqlDataSource.FederationConfiguration{
				Enabled:    true,
				ServiceSDL: sdl,
			},
		}

		dataSourceConfigs = append(dataSourceConfigs, dataSourceConfig)
	}

	return dataSourceConfigs
}

func (d *DatasourcePollerPoller) fetchServiceSDL(ctx context.Context, serviceURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceURL, bytes.NewReader([]byte(ServiceDefinitionQuery)))
	req.Header.Add("Content-Type", "application/json")

	if err != nil {
		return "", fmt.Errorf("create request: %v", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %v", err)
	}

	defer resp.Body.Close()

	var result struct {
		Data struct {
			Service struct {
				SDL string `json:"sdl"`
			} `json:"_service"`
		} `json:"data"`
		Errors GQLErr `json:"errors,omitempty"`
	}

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read bytes: %v", err)
	}

	if err = json.NewDecoder(bytes.NewReader(bs)).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %v", err)
	}

	if result.Errors != nil {
		return "", fmt.Errorf("response error:%v", result.Errors)
	}

	return result.Data.Service.SDL, nil
}
