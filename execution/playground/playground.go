// Package playground is a http.Handler hosting the GraphQL Playground application.
package playground

import (
	"embed"
	"html/template"
	"net/http"
	"path"
	"strings"
)

const (
	playgroundTemplate  = "playgroundTemplate"
	contentTypeHeader   = "Content-Type"
	contentTypeTextHTML = "text/html"
)

//go:embed files/*
var files embed.FS

// Config is the configuration Object to instruct ConfigureHandlers on how to setup all the http Handlers for the playground
type Config struct {
	// PathPrefix is a prefix you intend to put in front of all handlers
	PathPrefix string
	// PlaygroundPath is the Path where the playground website should be hosted
	PlaygroundPath string
	// GraphqlEndpointPath is the Path where the http Handler for synchronous (Query,Mutation) GraphQL requests should be hosted
	GraphqlEndpointPath string
	// GraphQLSubscriptionEndpointPath is the Path where the http Handler for asynchronous (Subscription) GraphQL requests should be hosted
	GraphQLSubscriptionEndpointPath string
}

type playgroundTemplateData struct {
	EndpointURL             string
	SubscriptionEndpointURL string
}

// HandlerConfig is the configuration Object for playground http Handlers
type HandlerConfig struct {
	// Path is where the handler should be hosted
	Path string
	// Handler is the http.HandlerFunc that should be hosted on the corresponding Path
	Handler http.HandlerFunc
}

// Handlers is an array of HandlerConfig
// The playground expects that you make all assigned Handlers available on the corresponding Path
type Handlers []HandlerConfig

func (h *Handlers) add(path string, handler http.HandlerFunc) {
	*h = append(*h, HandlerConfig{
		Path:    path,
		Handler: handler,
	})
}

// Playground manages the configuration of all HTTP handlers responsible for serving the GraphQL Playground
type Playground struct {
	cfg  Config
	data playgroundTemplateData
}

// New creates a Playground for given Config
func New(config Config) *Playground {
	data := playgroundTemplateData{
		EndpointURL:             config.GraphqlEndpointPath,
		SubscriptionEndpointURL: config.GraphQLSubscriptionEndpointPath,
	}

	return &Playground{
		cfg:  config,
		data: data,
	}
}

// Handlers configures and returns all Handlers for the Playground
func (p *Playground) Handlers() (handlers Handlers, err error) {
	handlers = make(Handlers, 0, 1)

	if err = p.configurePlaygroundHandler(&handlers); err != nil {
		return
	}

	return
}

func (p *Playground) configurePlaygroundHandler(handlers *Handlers) (err error) {
	playgroundHTML, err := files.ReadFile("files/playground.html")
	if err != nil {
		return
	}
	templates, err := template.New(playgroundTemplate).Parse(string(playgroundHTML))
	if err != nil {
		return
	}

	playgroundURL := path.Join(p.cfg.PathPrefix, p.cfg.PlaygroundPath)
	if playgroundURL != "/" && (playgroundURL == "" || strings.HasSuffix(p.cfg.PlaygroundPath, "/") ||
		(p.cfg.PlaygroundPath == "" && strings.HasSuffix(p.cfg.PathPrefix, "/"))) {
		playgroundURL += "/"
	}

	handlers.add(playgroundURL, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add(contentTypeHeader, contentTypeTextHTML)

		if err := templates.ExecuteTemplate(writer, playgroundTemplate, p.data); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(err.Error()))
		}
	})

	return nil
}
