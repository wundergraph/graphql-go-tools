package cmd

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware"
	"github.com/jensneuse/graphql-go-tools/pkg/proxy/http"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// staticProxyCmd represents the staticProxy command
var staticProxyCmd = &cobra.Command{
	Use:   "staticProxy",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("staticProxy called")

		staticProxy := http.NewStaticProxy(http.StaticProxyConfig{
			MiddleWares: []middleware.GraphqlMiddleware{
				&middleware.ContextMiddleware{},
			},
			Schema:     []byte(testSchema),
			BackendURL: "http://0.0.0.0:8889/query",
		})

		err := staticProxy.ListenAndServe("0.0.0.0:8888")
		if err != nil {
			cmd.OutOrStderr().Write([]byte(err.Error()))
		}
	},
}

func init() {
	rootCmd.AddCommand(staticProxyCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// staticProxyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// staticProxyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func printMemory() {
	go func() {
		for {
			time.Sleep(1 * time.Second)
			PrintMemUsage()
		}
	}()
}

const testSchema = `
directive @addArgumentFromContext(
	name: String!
	contextKey: String!
) on FIELD_DEFINITION

scalar String

schema {
	query: Query
}

type Query {
	documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
}

type Document {
	owner: String
	sensitiveInformation: String
}
`

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
