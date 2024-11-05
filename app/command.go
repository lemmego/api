package app

import "github.com/spf13/cobra"

type Command func(a AppManager) *cobra.Command

var rootCmd = &cobra.Command{}
