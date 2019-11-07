//go:generate packr
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

type Config struct {
	URLPrefix                   string
	PlaygroundURL               string
	GraphqlEndpoint             string
	GraphQLSubscriptionEndpoint string
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

func ConfigureHandlers(mux *http.ServeMux, config Config) error {
	box := packr.NewBox("./files")
	playgroundHTML, err := box.FindString("playground.html")
	if err != nil {
		return err
	}
	templates, err := template.New(playgroundTemplate).Parse(playgroundHTML)
	if err != nil {
		return err
	}

	playgroundURL := path.Join(config.URLPrefix, config.PlaygroundURL)
	data := playgroundTemplateData{
		CssURL:                  path.Join(config.URLPrefix, "playground.css"),
		JsURL:                   path.Join(config.URLPrefix, "playground.js"),
		FavIconURL:              path.Join(config.URLPrefix, "favicon.png"),
		LogoURL:                 path.Join(config.URLPrefix, "logo.png"),
		EndpointURL:             config.GraphqlEndpoint,
		SubscriptionEndpointURL: config.GraphQLSubscriptionEndpoint,
	}

	mux.HandleFunc(playgroundURL, func(writer http.ResponseWriter, request *http.Request) {
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
		if err := configureFileHandler(mux, box, files[i].fileName, files[i].fileURL, files[i].fileContentType); err != nil {
			return err
		}
	}

	return nil
}

func configureFileHandler(mux *http.ServeMux, box packr.Box, fileName, fileURL, contentType string) error {

	data, err := box.Find(fileName)
	if err != nil {
		return err
	}

	mux.HandleFunc(fileURL, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add(contentTypeHeader, contentType)
		_, _ = writer.Write(data)
	})

	return nil
}
