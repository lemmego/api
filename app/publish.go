package app

import (
	"bytes"
	"fmt"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
	"slices"
	"strings"
)

var tagsFlag string

var publishCmd = &cobra.Command{Use: "publish"}

type Publishable struct {
	FilePath string
	Content  []byte
	Tag      string
}

func (p *Publishable) Publish() error {
	filePath := p.FilePath

	if _, err := os.Stat(filePath); err != nil {
		// Define the substring to search for in the first line
		substring := "//go:build"

		// Find the index of the first newline character
		index := bytes.IndexByte(p.Content, '\n')
		if index != -1 {
			// Check if the first line contains the substring
			if bytes.Contains(p.Content[:index], []byte(substring)) {
				// Slice the byte array to remove the first line, including the newline
				p.Content = p.Content[index+1:]
			}
		}
		err := os.WriteFile(filePath, p.Content, 0644)
		if err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

func init() {
	publishCmd.PersistentFlags().StringVar(&tagsFlag, "tags", "", "Comma-separated tag names of package assets")
}

func publish(a *Application, publishables []*Publishable) *cobra.Command {
	publishCmd.Run = func(cmd *cobra.Command, args []string) {
		tags := []string{}
		if tagsFlag != "" {
			tags = strings.Split(tagsFlag, ",")
		}

		for _, publishable := range publishables {
			if len(tags) > 0 && slices.Contains(tags, publishable.Tag) {
				slog.Info(fmt.Sprintf("Publishing assets with tag %s", publishable.Tag))
				if err := publishable.Publish(); err != nil {
					panic(err)
				}
				slog.Info(fmt.Sprintf("Published file to %s", publishable.FilePath))
			} else {
				if err := publishable.Publish(); err != nil {
					panic(err)
				}
			}
		}
	}
	return publishCmd
}
