package cmd

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/codegen"
	"github.com/jensneuse/graphql-go-tools/pkg/imports"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/spf13/cobra"
	"io"
	"os"
)

var (
	filesRegex            string
	packageName           string
	outFile               string
	directiveStructSuffix string
)

// directiveUnmarshalCodeCmd represents the directiveUnmarshalCode command
var directiveUnmarshalCodeCmd = &cobra.Command{
	Use:   "directiveUnmarshalCode",
	Short: "Generates go code to unmarshal directives",
	Long: `directiveUnmarshalCode is a cli to generate code to unmarshal directives, input object type definitions, scalars and enums into go code
This is a convenient helper to make working with configurations attached to a GraphQL schema using directives easier.`,
	Example: `graphql-go-tools gen directiveUnmarshalCode -f ./pkg/codegen/testdata/schema.graphql -p main`,
	RunE: func(cmd *cobra.Command, args []string) error {
		scanner := &imports.Scanner{}
		file, err := scanner.ScanRegex(filesRegex)
		if err != nil {
			return err
		}

		buf := bytes.Buffer{}
		err = file.Render(false, &buf)
		if err != nil {
			return err
		}

		parser := astparser.NewParser()
		doc := ast.NewDocument()
		doc.Input.ResetInputBytes(buf.Bytes())
		rep := &operationreport.Report{}

		parser.Parse(doc, rep)
		if rep.HasErrors() {
			return rep
		}

		var out io.Writer
		if outFile == "" {
			out = os.Stdout
		} else {
			o, err := os.Create(outFile)
			if err != nil {
				return err
			}
			defer o.Close()
			out = o
		}

		config := codegen.Config{
			PackageName:           packageName,
			DirectiveStructSuffix: directiveStructSuffix,
		}

		gen := codegen.New(doc, config)
		_, err = gen.Generate(out)
		return err
	},
}

func init() {
	genCmd.AddCommand(directiveUnmarshalCodeCmd)

	directiveUnmarshalCodeCmd.Flags().StringVarP(&filesRegex, "filesRegex", "f", "", "filesRegex is a regex to specify all the files the generator should use (required)")
	_ = directiveUnmarshalCodeCmd.MarkFlagRequired("filesRegex")

	directiveUnmarshalCodeCmd.Flags().StringVarP(&packageName, "packageName", "p", "", "packageName is the package for the generated code (required)")
	_ = directiveUnmarshalCodeCmd.MarkFlagRequired("packageName")

	directiveUnmarshalCodeCmd.Flags().StringVarP(&directiveStructSuffix, "directiveStructSuffix", "s", "", "directiveStructSuffix is the suffix which gets appended to all directive struct names to avoid naming collisions (optional)")

	directiveUnmarshalCodeCmd.Flags().StringVarP(&outFile, "outFile", "o", "", "outFile is a flag to redirect the output directly into a file (optional)")
}
