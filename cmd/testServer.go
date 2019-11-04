/*
Copyright © 2019 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/execution"
	"github.com/jensneuse/graphql-go-tools/pkg/playground"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"io/ioutil"
	"log"
	"net/http"
)

var (
	schemaFile string
)

// testServerCmd represents the testServer command
var testServerCmd = &cobra.Command{
	Use:   "testServer",
	Short: "starts a graphQL test server",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("starting testServer...")
		startTestServer()
	},
}

func init() {
	rootCmd.AddCommand(testServerCmd)
	testServerCmd.PersistentFlags().StringVar(&schemaFile, "schema", "./schema.graphql", "schema is the configuration file")
	_ = viper.BindPFlag("schema", rootCmd.PersistentFlags().Lookup("schema"))
}

func startTestServer() {
	mux := http.NewServeMux()
	graphqlEndpoint := "/graphql"
	schemaData, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		log.Fatal(err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync() // nolint

	handler, err := execution.NewHandler(schemaData, logger)
	if err != nil {
		log.Fatal(err)
	}
	mux.HandleFunc(graphqlEndpoint, func(writer http.ResponseWriter, request *http.Request) {
		buf := bytes.NewBuffer(make([]byte, 0, 4096))
		err := handler.Handle(request.Body, buf)
		if err != nil {
			err := json.NewEncoder(writer).Encode(struct {
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors"`
			}{
				Errors: []struct {
					Message string `json:"message"`
				}{
					{
						Message: err.Error(),
					},
				},
			})
			if err != nil {
				log.Fatal(err)
			}
			return
		}
		writer.Header().Add("Content-Type", "application/json")
		_, _ = buf.WriteTo(writer)
	})
	err = playground.ConfigureHandlers(mux, playground.Config{
		URLPrefix:       "/playground",
		PlaygroundURL:   "",
		GraphqlEndpoint: graphqlEndpoint,
	})
	if err != nil {
		log.Fatal(err)
		return
	}
	addr := ":9111"
	fmt.Printf("Listening on: %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
