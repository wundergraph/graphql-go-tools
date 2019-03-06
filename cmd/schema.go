package cmd

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/printer"
	"github.com/spf13/cobra"
	"io/ioutil"
)

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:     "schema",
	Short:   "schema formats a graphql schema file to sdt out",
	Example: "fmt schema starwars.schema.graphql > formatted.graphql",
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) != 1 {
			return fmt.Errorf("schema: must provide 1 arg (fileName)")
		}

		fileName := args[0]

		data, err := ioutil.ReadFile(fileName)
		if err != nil {
			return err
		}

		p := parser.NewParser()
		err = p.ParseTypeSystemDefinition(data)
		if err != nil {
			return err
		}

		l := lookup.New(p, 8)
		w := lookup.NewWalker(1024, 8)
		w.SetLookup(l)
		w.WalkTypeSystemDefinition()

		astPrinter := printer.New()
		astPrinter.SetInput(p, l, w)

		astPrinter.PrintTypeSystemDefinition(cmd.OutOrStdout())

		return nil
	},
}

func init() {
	fmtCmd.AddCommand(schemaCmd)
}
