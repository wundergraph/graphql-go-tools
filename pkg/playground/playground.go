//go:generate packr

// Package playground is a http.Handler hosting the GraphQL Playground application.
package playground

import (
	"github.com/gobuffalo/packr"
	"html/template"
	"net/http"
	"path"
)

const (
	playgroundTemplate = "playgroundTemplate"
)

const (
	contentTypeHeader         = "Content-Type"
	contentTypeImagePNG       = "image/png"
	contentTypeTextHTML       = "text/html"
	contentTypeTextCSS        = "text/css"
	contentTypeTextJavascript = "text/javascript"
)

// Config is the configuration Object to instruct ConfigureHandlers on how to setup all the http Handlers for the playground
type Config struct {
	// URLPrefix is a prefix you intend to put in front of all handlers
	URLPrefix                   string
	// PlaygroundURL is the URL where the playground website should be hosted
	PlaygroundURL               string
	// GraphqlEndpointURL is the URL where the http Handler for synchronous (Query,Mutation) GraphQL requests should be hosted
	GraphqlEndpointURL             string
	// GraphqlEndpointURL is the URL where the http Handler for asynchronous (Subscription) GraphQL requests should be hosted
	GraphQLSubscriptionEndpointURL string
}

type playgroundTemplateData struct {
	CssURL                  string
	JsURL                   string
	FavIconURL              string
	LogoURL                 string
	EndpointURL             string
	SubscriptionEndpointURL string
}

type fileConfig struct {
	fileName        string
	fileURL         string
	fileContentType string
}

// HandlerConfig is the configuration Object for all playground http Handlers
type HandlerConfig struct {
	// URL is where the handler should be hosted
	URL     string
	// Handler is the http.HandlerFunc that should be hosted on the corresponding URL
	Handler http.HandlerFunc
}

// Handlers is an array of HandlerConfig
// The playground expects that you make all assigned Handlers available on the corresponding URL
type Handlers []HandlerConfig

func (h *Handlers) add(url string, handler http.HandlerFunc) {
	*h = append(*h, HandlerConfig{
		URL:     url,
		Handler: handler,
	})
}

// ConfigureHandlers takes your Config and sets up all handlers to the supplied Handlers slice
func ConfigureHandlers(config Config, handlers *Handlers) (err error) {

	box := packr.NewBox("./files")
	playgroundHTML, err := box.FindString("playground.html")
	if err != nil {
		return
	}
	templates, err := template.New(playgroundTemplate).Parse(playgroundHTML)
	if err != nil {
		return
	}

	playgroundURL := path.Join(config.URLPrefix, config.PlaygroundURL)
	data := playgroundTemplateData{
		CssURL:                  path.Join(config.URLPrefix, "playground.css"),
		JsURL:                   path.Join(config.URLPrefix, "playground.js"),
		FavIconURL:              path.Join(config.URLPrefix, "favicon.png"),
		LogoURL:                 path.Join(config.URLPrefix, "logo.png"),
		EndpointURL:             config.GraphqlEndpointURL,
		SubscriptionEndpointURL: config.GraphQLSubscriptionEndpointURL,
	}

	handlers.add(playgroundURL, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add(contentTypeHeader, contentTypeTextHTML)
		err := templates.ExecuteTemplate(writer, playgroundTemplate, data)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(err.Error()))
		}
	})

	files := []fileConfig{
		{
			fileName:        "playground.css",
			fileURL:         data.CssURL,
			fileContentType: contentTypeTextCSS,
		},
		{
			fileName:        "playground.js",
			fileURL:         data.JsURL,
			fileContentType: contentTypeTextJavascript,
		},
		{
			fileName:        "favicon.png",
			fileURL:         data.FavIconURL,
			fileContentType: contentTypeImagePNG,
		},
		{
			fileName:        "logo.png",
			fileURL:         data.LogoURL,
			fileContentType: contentTypeImagePNG,
		},
	}

	for i := 0; i < len(files); i++ {
		err = configureFileHandler(handlers, box, files[i].fileName, files[i].fileURL, files[i].fileContentType)
		if err != nil {
			return err
		}
	}

	return nil
}

func configureFileHandler(handlers *Handlers, box packr.Box, fileName, fileURL, contentType string) error {
	data, err := box.Find(fileName)
	if err != nil {
		return err
	}

	handlers.add(fileURL,func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add(contentTypeHeader, contentType)
		_, _ = writer.Write(data)
	})

	return nil
}
