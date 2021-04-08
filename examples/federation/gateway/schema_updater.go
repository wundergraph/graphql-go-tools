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

	graphqlDataSource "github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
)

type UpdateDatasourceHandler interface {
	UpdateDatasource(dataSourceConfig ...graphqlDataSource.Configuration)
}

type UpdateDatasourceHandlerFn func(dataSourceConfig ...graphqlDataSource.Configuration)

func (u UpdateDatasourceHandlerFn) UpdateDatasource(dataSourceConfig ...graphqlDataSource.Configuration) {
	u(dataSourceConfig...)
}

type ServiceConfig struct {
	Name string
	URL  string
	WS   string
}

type DatasourceWatcherConfig struct {
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

func NewDatasourceWatcher(
	httpClient *http.Client,
	config DatasourceWatcherConfig,
) *DatasourceWatcher {
	return &DatasourceWatcher{
		httpClient:                httpClient,
		mu:                        &sync.Mutex{},
		config:                    config,
		sdlMap:                    make(map[string]string),
	}
}

type DatasourceWatcher struct {
	httpClient *http.Client

	mu     *sync.Mutex
	config DatasourceWatcherConfig
	sdlMap map[string]string

	updateDatasourceCallbacks []UpdateDatasourceHandler
}

func (d *DatasourceWatcher) Register(updateDatasourceHandler UpdateDatasourceHandler) {
	d.updateDatasourceCallbacks = append(d.updateDatasourceCallbacks, updateDatasourceHandler)
}

func (d *DatasourceWatcher) Start(ctx context.Context) {
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

func (d *DatasourceWatcher) UpdateServiceSDL(newServiceConfig ServiceConfig, sdl string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.config.Services {
		if d.config.Services[i].Name != newServiceConfig.Name {
			continue
		}

		d.config.Services[i] = newServiceConfig
		d.sdlMap[newServiceConfig.Name] = sdl

		return
	}

	d.config.Services = append(d.config.Services, newServiceConfig)
	d.sdlMap[newServiceConfig.Name] = sdl

}

func (d *DatasourceWatcher) updateSDLs(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.sdlMap = make(map[string]string)

	for _, serviceConf := range d.config.Services {
		sdl, err := d.fetchServiceSDL(ctx, serviceConf.URL)
		if err != nil {
			log.Println("Failed to get sdl", err)
		}

		d.sdlMap[serviceConf.Name] = sdl
	}

	d.updateCallbacks()
}

func (d *DatasourceWatcher) updateCallbacks() {
	dataSourceConfigs := d.createDatasourceConfig()

	for i := range d.updateDatasourceCallbacks {
		d.updateDatasourceCallbacks[i].UpdateDatasource(dataSourceConfigs...)
	}
}

func (d *DatasourceWatcher) createDatasourceConfig() []graphqlDataSource.Configuration {
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

func (d *DatasourceWatcher) fetchServiceSDL(ctx context.Context, serviceURL string) (string, error) {
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

	fmt.Println("resp.Body", string(bs))
	if err = json.NewDecoder(bytes.NewReader(bs)).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %v", err)
	}

	if result.Errors != nil {
		return "", fmt.Errorf("response error:%v", result.Errors)
	}

	return result.Data.Service.SDL, nil
}
