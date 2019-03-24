package cmd

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy/http"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"
	"io/ioutil"
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
	staticProxyBackendAddr string
	staticProxyBackendURL  string
)

// staticProxyCmd represents the staticProxy command
var staticProxyCmd = &cobra.Command{
	Use:   "staticProxy",
	Short: "staticProxy starts a proxy that cannot be configured at runtime",
	Long: `staticProxy is a simple graphql proxy that sits in front of a single graphql backend
In the current version staticProxy is capable of rewriting requests with the context middleware.
See the following docs for usage details:
https://jens-neuse.gitbook.io/graphql-go-tools/middlewares/context-middleware`,
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
- backendAddr: %s
- backendURL: %s

Schema Configuration:
- schema file: %s

Debug Configuration:
- pprof enabled: %t
- pprofAddr: %s
- print memory: %t

Example usage:
curl --data '{"operationName":null,"variables":{},"query":"{\n  documents{\n    owner\n    sensitiveInformation\n  }\n}\n"}' --header "Content-Type: application/json" -v http://0.0.0.0:8888

`,
		staticProxyAddr,
		staticProxyBackendAddr,
		staticProxyBackendURL,
		staticProxySchemaFile,
		staticProxyRunPPROF,
		staticProxyPprofAddr,
		staticProxyPrintMemory)
}

func runProxyBlocking() {

	schema, err := ioutil.ReadFile(staticProxySchemaFile)
	if err != nil {
		panic(err)
	}

	prox := http.NewFastStaticProxy(http.FastStaticProxyConfig{
		MiddleWares: []middleware.GraphqlMiddleware{
			&middleware.ContextMiddleware{},
		},
		Schema:      schema,
		BackendURL:  staticProxyBackendURL,
		BackendHost: staticProxyBackendAddr,
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
	staticProxyCmd.Flags().StringVar(&staticProxyBackendAddr, "backendAddr", "0.0.0.0:8080", "the backend Addr")
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
