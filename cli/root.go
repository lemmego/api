package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Create a new command
var RootCmd = &cobra.Command{
	Use:   "",
	Short: fmt.Sprintf("%s", os.Getenv("APP_NAME")),
}

// Execute the command
func Execute() error {
	genCmd.AddCommand(handlerCmd)
	genCmd.AddCommand(migrationCmd)
	genCmd.AddCommand(modelCmd)
	genCmd.AddCommand(inputCmd)
	genCmd.AddCommand(formCmd)

	RootCmd.AddCommand(genCmd)

	return RootCmd.Execute()
}
