package app

import (
	"encoding/json"
	"fmt"

	"github.com/lemmego/api/utils"
	"github.com/spf13/cobra"
)

// ProjectPaths is where generated code goes, as the application resolves it.
//
// The field names are the config keys, so the JSON this produces can be read
// without knowing anything about the struct.
type ProjectPaths struct {
	ConfigPath     string `json:"config_path"`
	CommandPath    string `json:"command_path"`
	HandlerPath    string `json:"handler_path"`
	InputPath      string `json:"input_path"`
	MiddlewarePath string `json:"middleware_path"`
	MigrationPath  string `json:"migration_path"`
	ModelPath      string `json:"model_path"`
	RoutePath      string `json:"route_path"`
}

// ResolvedPaths reports where this application puts generated code.
func ResolvedPaths() ProjectPaths {
	return ProjectPaths{
		ConfigPath:     utils.ConfigPath(),
		CommandPath:    utils.CommandPath(),
		HandlerPath:    utils.HandlerPath(),
		InputPath:      utils.InputPath(),
		MiddlewarePath: utils.MiddlewarePath(),
		MigrationPath:  utils.MigrationPath(),
		ModelPath:      utils.ModelPath(),
		RoutePath:      utils.RoutePath(),
	}
}

// pathsCmd prints the resolved paths as JSON.
//
// It exists so the CLI's generators can put a file where this project says it
// belongs. The CLI runs as its own binary and cannot read internal/configs,
// which is Go source only this application can execute — so it asks rather
// than guesses, and a project that moved its models to ./domain/models gets
// its next generated model there.
var pathsCmd = &cobra.Command{
	Use:    "config:paths",
	Short:  "Print the paths this project generates code into",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		encoded, err := json.Marshal(ResolvedPaths())
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return nil
	},
}
