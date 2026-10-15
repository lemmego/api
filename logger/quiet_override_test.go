package logger

import "testing"

// Quiet mode raises a command's console to warn and keeps the file at full
// level, which is right for `migrate up`: an operator is reading the output
// and boot chatter buries the answer.
//
// It is wrong for a worker. `tasker:work` and `tasker:schedule` are
// long-running servers that happen to be spelled as subcommands — nobody is
// reading a console, their whole purpose is to report what they did, and on a
// container platform the file quiet mode preserves sits in an ephemeral
// filesystem nobody will ever open. The symptom is a deployment whose queue
// dashboard shows jobs cycling while its log is empty.
//
// LOG_QUIET is how such a process asks for its voice back, so the decision
// stops being one the framework makes from the shape of the invocation alone.
func TestLogQuietOverridesTheConsoleDefault(t *testing.T) {
	for name, test := range map[string]struct {
		value   string
		console bool // what RunningInConsole would say
		want    bool
	}{
		"a command is quiet by default":         {"", true, true},
		"a server is not":                       {"", false, false},
		"a command can ask to be heard":         {"false", true, false},
		"a server can ask to be quietened":      {"true", false, true},
		"an unparsable value keeps the default": {"maybe", true, true},
	} {
		t.Run(name, func(t *testing.T) {
			if test.value != "" {
				t.Setenv(EnvQuiet, test.value)
			}
			env := &environment{}

			if got := env.boolean(EnvQuiet, test.console); got != test.want {
				t.Errorf("LOG_QUIET=%q with RunningInConsole=%v resolved to %v, want %v",
					test.value, test.console, got, test.want)
			}
		})
	}
}
