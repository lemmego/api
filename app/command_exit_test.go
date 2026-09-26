package app

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

// A command that fails must report its error, not panic.
//
// Cobra prints the message and the usage itself, so panicking printed it a
// second time under a goroutine dump — a mistyped command or an ordinary
// expected failure looked like the framework had crashed. The caller also
// needs the error back so it can shut down cleanly before exiting.
func TestRegisterCommandsReturnsCommandErrors(t *testing.T) {
	previous := rootCmd
	t.Cleanup(func() { rootCmd = previous })

	wantErr := errors.New("the cache driver cannot be reached from here")
	rootCmd = &cobra.Command{Use: "test"}
	rootCmd.SetArgs([]string{"failing"})
	rootCmd.SetOut(nopWriter{})
	rootCmd.SetErr(nopWriter{})

	a := &application{commands: []Command{
		func(App) *cobra.Command {
			return &cobra.Command{
				Use:  "failing",
				RunE: func(*cobra.Command, []string) error { return wantErr },
			}
		},
	}}

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("a failing command panicked instead of reporting: %v", r)
			}
		}()
		err = a.registerCommands()
	}()

	if !errors.Is(err, wantErr) {
		t.Fatalf("registerCommands() error = %v, want %v", err, wantErr)
	}
}

func TestRegisterCommandsReturnsNilOnSuccess(t *testing.T) {
	previous := rootCmd
	t.Cleanup(func() { rootCmd = previous })

	rootCmd = &cobra.Command{Use: "test"}
	rootCmd.SetArgs([]string{"working"})
	rootCmd.SetOut(nopWriter{})
	rootCmd.SetErr(nopWriter{})

	ran := false
	a := &application{commands: []Command{
		func(App) *cobra.Command {
			return &cobra.Command{
				Use:  "working",
				RunE: func(*cobra.Command, []string) error { ran = true; return nil },
			}
		},
	}}

	if err := a.registerCommands(); err != nil {
		t.Fatalf("registerCommands() error = %v", err)
	}
	if !ran {
		t.Error("the command did not run")
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
