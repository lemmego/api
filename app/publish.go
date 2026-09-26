package app

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var tagsFlag string

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Copy assets a package ships into this project",
	Long: "Copy assets a package ships — configuration, migrations, views — into this\n" +
		"project, where they can be read and edited. An existing file is never\n" +
		"overwritten, so publishing twice is safe and the second run reports what it\n" +
		"left alone.",
}

// Publishable is one file a package offers to copy into the project.
//
// The file belongs to the project once published: it is not overwritten on a
// later run, so an edit survives an upgrade. That also means a package cannot
// push out a fix to a file someone has already taken, which is why anything
// a package needs to keep control of belongs in the package rather than here.
type Publishable struct {
	// FilePath is where the file lands, relative to the project root.
	// Parent directories are created as needed.
	FilePath string

	Content []byte

	// Tag groups related files so `publish --tags` can select them, e.g.
	// "config", "migrations", "views".
	Tag string
}

// Publish writes the file unless it already exists, reporting whether it
// wrote.
//
// The bool matters: a skip used to be indistinguishable from a write, both
// returning a nil error and logging nothing, so someone who had published
// once and then upgraded had no way to tell that the new version of a file
// had not arrived.
func (p *Publishable) Publish() (bool, error) {
	if _, err := os.Stat(p.FilePath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if dir := filepath.Dir(p.FilePath); dir != "" && dir != "." {
		// Without this a publishable targeting a directory the project does
		// not have yet fails on the write, which read as a bug in the
		// package rather than a missing directory.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	content := p.Content
	// A build constraint keeps a stub out of the package's own build. It is
	// not wanted in the copy, which is ordinary project code.
	if index := bytes.IndexByte(content, '\n'); index != -1 {
		if bytes.Contains(content[:index], []byte("//go:build")) {
			content = content[index+1:]
		}
	}

	if err := os.WriteFile(p.FilePath, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func init() {
	publishCmd.PersistentFlags().StringVar(&tagsFlag, "tags", "",
		"Comma-separated tags selecting which assets to publish; all of them when omitted")
}

// Tags lists the distinct tags across publishables, so the command can tell
// someone what they could have asked for.
func publishableTags(publishables []*Publishable) []string {
	var tags []string
	for _, publishable := range publishables {
		if publishable.Tag != "" && !slices.Contains(tags, publishable.Tag) {
			tags = append(tags, publishable.Tag)
		}
	}
	slices.Sort(tags)
	return tags
}

func publish(publishables []*Publishable) *cobra.Command {
	publishCmd.RunE = func(cmd *cobra.Command, args []string) error {
		var tags []string
		if trimmed := strings.TrimSpace(tagsFlag); trimmed != "" {
			for _, tag := range strings.Split(trimmed, ",") {
				if tag = strings.TrimSpace(tag); tag != "" {
					tags = append(tags, tag)
				}
			}
		}

		// --tags used to be decorative: a tag that did not match still fell
		// through to an else branch that published the file anyway, so the
		// flag narrowed nothing.
		if len(tags) > 0 {
			available := publishableTags(publishables)
			for _, tag := range tags {
				if !slices.Contains(available, tag) {
					return fmt.Errorf("no assets are tagged %q; available tags: %s",
						tag, strings.Join(available, ", "))
				}
			}
		}

		var written, skipped int
		for _, publishable := range publishables {
			if len(tags) > 0 && !slices.Contains(tags, publishable.Tag) {
				continue
			}
			ok, err := publishable.Publish()
			if err != nil {
				return fmt.Errorf("publishing %s: %w", publishable.FilePath, err)
			}
			if ok {
				written++
				slog.Info("published", "path", publishable.FilePath, "tag", publishable.Tag)
			} else {
				skipped++
				slog.Info("already present, left alone", "path", publishable.FilePath, "tag", publishable.Tag)
			}
		}

		cmd.Printf("Published %d file(s), left %d already present.\n", written, skipped)
		return nil
	}
	return publishCmd
}
