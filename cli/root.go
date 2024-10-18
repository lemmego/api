package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var shouldRunInteractively = false

// rootCmd is the top-level command, which will
// hold all the subcommands such as gen, or any package-level
// commands installed via service providers.
var rootCmd = &cobra.Command{
	Use:     "lemmego",
	Aliases: []string{"lmg"},
	Short:   fmt.Sprintf("%s", os.Getenv("APP_NAME")),
}

// AddCmd adds a new sub-command to the root command.
func AddCmd(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

// Execute the command and register the sub-commands.
func Execute() error {
	genCmd.PersistentFlags().BoolVarP(&shouldRunInteractively, "interactive", "i", false, "Run interactively")

	genCmd.AddCommand(handlerCmd)
	genCmd.AddCommand(migrationCmd)
	genCmd.AddCommand(modelCmd)
	genCmd.AddCommand(inputCmd)
	genCmd.AddCommand(formCmd)

	AddCmd(genCmd)

	return rootCmd.Execute()
}
