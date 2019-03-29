package cmd

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy"
	fastproxy "github.com/jensneuse/graphql-go-tools/pkg/proxy/fasthttp"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"
	"io/ioutil"
	"net/url"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

var (
	staticProxyPrintMemory bool
	staticProxyRunPPROF    bool

	staticProxyAddr        string
	staticProxyPprofAddr   string
	staticProxySchemaFile  string
	staticProxyBackendURL  string
	staticProxyContextKeys []string
)

// staticProxyCmd represents the staticProxy command
var staticProxyCmd = &cobra.Command{
	Use:   "staticProxy",
	Short: "staticProxy starts a proxy that's configurable upfront, just like nginx",
	Long: `staticProxy is a simple graphql proxy that sits in front of a single graphql backend.

In the current version staticProxy is capable of rewriting requests with the context middleware.
See the following docs for usage details:
https://jens-neuse.gitbook.io/graphql-go-tools/middlewares/context-middleware

A dynamicProxy is in the making.
In contrast to the staticProxy the dynamic proxy can be configured at runtime.`,
	Example: "staticProxy --schemaFile schema.graphql --contextKeys token,user",
	Run: func(cmd *cobra.Command, args []string) {
		printConfig()
		runPrintMemoryUsage()
		runPPROF()
		runProxyBlocking()
	},
}

func printConfig() {
	fmt.Printf(`Running static proxy..

Proxy Configuration:
- proxyAddr: %s
- backendURL: %s

Schema Configuration:
- schema file: %s

Debug Configuration:
- pprof enabled: %t
- pprofAddr: %s
- print memory: %t

Headers that will be added to the context:
%s

Example usage:
curl --data '{"operationName":null,"variables":{},"query":"{documents{owner sensitiveInformation}}"}' --header 'user: "jens"' --header 'Content-Type: application/json' -v http://0.0.0.0:8888 | jq

`,
		staticProxyAddr,
		staticProxyBackendURL,
		staticProxySchemaFile,
		staticProxyRunPPROF,
		staticProxyPprofAddr,
		staticProxyPrintMemory,
		staticProxyContextKeys)
}

func runProxyBlocking() {

	schema, err := ioutil.ReadFile(staticProxySchemaFile)
	if err != nil {
		panic(err)
	}

	addHeadersToContext := make([][]byte, len(staticProxyContextKeys))
	for _, value := range staticProxyContextKeys {
		addHeadersToContext = append(addHeadersToContext, []byte(value))
	}

	backendURL, err := url.Parse(staticProxyBackendURL)
	if err != nil {
		panic(err)
	}

	prox := fastproxy.NewFastStaticProxy(fastproxy.FastStaticProxyConfig{
		MiddleWares: []middleware.GraphqlMiddleware{
			&middleware.ValidationMiddleware{},
			&middleware.ContextMiddleware{},
		},
		RequestConfigProvider: proxy.NewStaticSchemaProvider(proxy.RequestConfig{
			Schema:              &schema,
			BackendURL:          *backendURL,
			AddHeadersToContext: addHeadersToContext,
		}),
	})

	err = prox.ListenAndServe(staticProxyAddr)
	if err != nil {
		panic(err)
	}
}

func runPPROF() {

	if !staticProxyRunPPROF {
		return
	}

	go func() {
		err := fasthttp.ListenAndServe(staticProxyPprofAddr, pprofhandler.PprofHandler)
		if err != nil {
			panic(err)
		}
	}()
}

func init() {
	rootCmd.AddCommand(staticProxyCmd)

	staticProxyCmd.Flags().BoolVar(&staticProxyPrintMemory, "printMemory", false, "continuously prints memory usage")
	staticProxyCmd.Flags().BoolVar(&staticProxyRunPPROF, "pprof", false, "run pprof server in background thread")
	staticProxyCmd.Flags().StringVar(&staticProxyAddr, "proxyAddr", "0.0.0.0:8888", "host:port the proxy should listen on")
	staticProxyCmd.Flags().StringVar(&staticProxyPprofAddr, "pprofAddr", "0.0.0.0:8081", "host:port the pprof web server should listen on")
	staticProxyCmd.Flags().StringVar(&staticProxySchemaFile, "schemaFile", "./schema.graphql", "the file to read the schema from")
	staticProxyCmd.Flags().StringVar(&staticProxyBackendURL, "backendURL", "http://0.0.0.0:8080/query", "the backend URL to proxy requests to")
	staticProxyCmd.Flags().StringSliceVar(&staticProxyContextKeys, "contextKeys", nil, "the keys that should be read from the header and set to the context")
}

func runPrintMemoryUsage() {

	if !staticProxyPrintMemory {
		return
	}

	go func() {
		for {
			time.Sleep(1 * time.Second)
			PrintMemUsage()
		}
	}()
}

func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
	fmt.Printf("\tStackSys = %v MiB", bToMb(m.StackSys))
	fmt.Printf("\tStackInUse = %v MiB", bToMb(m.StackInuse))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
