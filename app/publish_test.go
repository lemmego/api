package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishWritesAndThenLeavesAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "configs", "thing.go")
	p := &Publishable{FilePath: path, Content: []byte("original"), Tag: "config"}

	// The parent directory does not exist. Publishing used to fail here, and
	// the error read as a bug in the package rather than a missing directory.
	wrote, err := p.Publish()
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if !wrote {
		t.Fatal("the first publish reported that it wrote nothing")
	}

	if err := os.WriteFile(path, []byte("edited by the user"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := &Publishable{FilePath: path, Content: []byte("a newer version"), Tag: "config"}
	wrote, err = second.Publish()
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if wrote {
		t.Fatal("the second publish overwrote a file the user owns")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "edited by the user" {
		t.Errorf("content = %q; an edit must survive a later publish", content)
	}
}

// A build constraint keeps a stub out of its own package's build and is not
// wanted in the published copy.
func TestPublishStripsABuildConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stub.go")
	p := &Publishable{FilePath: path, Content: []byte("//go:build ignore\npackage configs\n")}
	if _, err := p.Publish(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "package configs\n" {
		t.Errorf("content = %q, want the constraint removed", content)
	}
}

// --tags used to be decorative: a tag that did not match still fell through
// to a branch that published the file anyway.
func TestPublishTagsSelectWhatIsWritten(t *testing.T) {
	dir := t.TempDir()
	config := &Publishable{FilePath: filepath.Join(dir, "config.go"), Content: []byte("c"), Tag: "config"}
	migration := &Publishable{FilePath: filepath.Join(dir, "migration.go"), Content: []byte("m"), Tag: "migrations"}

	cmd := publish([]*Publishable{config, migration})
	t.Cleanup(func() { tagsFlag = "" })

	tagsFlag = "config"
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(config.FilePath); err != nil {
		t.Errorf("the selected tag was not published: %v", err)
	}
	if _, err := os.Stat(migration.FilePath); !os.IsNotExist(err) {
		t.Error("an unselected tag was published anyway")
	}
}

// Asking for a tag nothing carries is a typo, and silently doing nothing is
// the least helpful possible answer.
func TestPublishRejectsAnUnknownTag(t *testing.T) {
	dir := t.TempDir()
	cmd := publish([]*Publishable{
		{FilePath: filepath.Join(dir, "config.go"), Content: []byte("c"), Tag: "config"},
	})
	t.Cleanup(func() { tagsFlag = "" })

	tagsFlag = "migration" // the real tag is "migrations"
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("an unknown tag was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "config.go")); !os.IsNotExist(statErr) {
		t.Error("an unknown tag still published something")
	}
}

func TestPublishWithoutTagsWritesEverything(t *testing.T) {
	dir := t.TempDir()
	first := &Publishable{FilePath: filepath.Join(dir, "a.go"), Content: []byte("a"), Tag: "config"}
	second := &Publishable{FilePath: filepath.Join(dir, "b.go"), Content: []byte("b"), Tag: "migrations"}

	cmd := publish([]*Publishable{first, second})
	t.Cleanup(func() { tagsFlag = "" })
	tagsFlag = ""

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*Publishable{first, second} {
		if _, err := os.Stat(p.FilePath); err != nil {
			t.Errorf("%s was not published: %v", p.FilePath, err)
		}
	}
}
