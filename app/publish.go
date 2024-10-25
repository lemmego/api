package app

import (
	"fmt"
	"github.com/spf13/cobra"
	"strings"
)

var publish string

func init() {
	publishCmd.PersistentFlags().StringVar(&publish, "publish", "", "Comma-separated tag names of package assets")
}

var publishCmd = &cobra.Command{
	Use: "",
	Run: func(cmd *cobra.Command, args []string) {
		publishables := strings.Split(publish, ",")
		fmt.Println("Publishing...")
		for _, publishable := range publishables {
			fmt.Println(publishable)
		}
	},
}
