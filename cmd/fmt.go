package cmd

import (
	"github.com/spf13/cobra"
)

// fmtCmd represents the fmt command
var fmtCmd = &cobra.Command{
	Use:   "fmt",
	Short: "fmt formats graphql document files to std out",
}

func init() {
	rootCmd.AddCommand(fmtCmd)
}
