package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Translator resolves a message key.
//
// It is declared here rather than imported from lemmego/i18n, for the reason
// deps_test.go gives for app.Disk: the core declares the seam and a provider
// supplies the implementation. i18n depends on golang.org/x/text for CLDR
// plural data and language tags, and a project that never registers the
// provider should not carry those tables to run a validator.
//
// It also avoids a module cycle. i18n/provider imports api/app, so an api
// that imported i18n would make the two mutually dependent — the shape the
// gpa-and-its-drivers cycle is already on the list to break.
//
// *i18n.Translator satisfies this structurally; it has more methods, which an
// interface does not mind. Deliberately no Locale() returning a
// language.Tag — that one signature would be the x/text import this
// interface exists to avoid.
type Translator interface {
	// Get returns the message for a key with its placeholders substituted,
	// or the key itself when there is no such message.
	Get(key string, repl ...map[string]any) string

	// Has reports whether a message exists, so a caller can choose between a
	// translated label and something else without the absence being recorded
	// as a problem.
	Has(key string) bool
}

// Localizer is the service the container holds: a translator that can be
// narrowed to one locale.
//
// A request's own translator is on the request context; this is what answers
// outside a request, and what a mail channel narrows once it knows a
// recipient's preference.
type Localizer interface {
	Translator
	For(locale string) Translator
}

// translatorKey carries the request's translator on the request context.
//
// A string rather than a typed key because ctx.Set and ctx.Get take strings,
// and prefixed so it cannot collide with an application's own value.
const translatorKey = "app.translator"

// ContextWithTranslator returns a context carrying a request's translator,
// where ctx.T finds it.
//
// A context.Context rather than a Context, because the middleware that
// resolves a locale has to wrap the mux — it runs before pattern matching and
// therefore before any app.Context exists.
func ContextWithTranslator(ctx context.Context, t Translator) context.Context {
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, translatorKey, t)
}

// TranslatorFromContext returns the translator a context carries, or nil.
func TranslatorFromContext(ctx context.Context) Translator {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(translatorKey).(Translator)
	return t
}

// BuiltinValidationMessages returns a copy of the English the framework falls
// back to when no translator is registered.
//
// Exported so that the one package importing both this module and
// lemmego/i18n can assert the two agree. The duplication is deliberate — it
// is what keeps api from depending on i18n — and a parity test is what keeps
// it from drifting.
func BuiltinValidationMessages() map[string]string {
	out := make(map[string]string, len(builtinMessages))
	for key, text := range builtinMessages {
		out[key] = text
	}
	return out
}

// Interpolate substitutes :name placeholders in a message.
//
// Exported for the same parity test, which drives it and i18n's own
// substituter over the same adversarial table.
func Interpolate(text string, repl map[string]any) string {
	return interpolate(text, repl)
}

// interpolate substitutes :name placeholders.
//
// The pairs are sorted longest-key-first because strings.NewReplacer prefers
// the first pair in argument order that matches at a position, where the PHP
// strtr this syntax comes from prefers the longest. Unsorted, with "value"
// handed over before "values", "one of :values" renders as the value of
// :value with a stray "s" after it — and both placeholders are in the
// validation catalogue below: contains uses :value and in uses :values.
//
// The replacer also never rescans its own output, which is what makes a
// replacement value containing a colon safe. That is load-bearing in exactly
// one place here: validation.file_prefixed substitutes a client-supplied
// filename, so a file named ":message.txt" stays literal. A loop of
// strings.ReplaceAll would be wrong on both counts.
//
// An unknown placeholder is left verbatim. ":nmae" in the page is a bug
// report; an empty space is a mystery.
func interpolate(text string, repl map[string]any) string {
	if len(repl) == 0 || !strings.ContainsRune(text, ':') {
		return text
	}

	keys := make([]string, 0, len(repl))
	for key := range repl {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if d := len(b) - len(a); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})

	pairs := make([]string, 0, len(repl)*2)
	for _, key := range keys {
		pairs = append(pairs, ":"+key, displayValue(repl[key]))
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// displayValue renders a replacement value.
//
// A []string joins with ", " rather than going through fmt's "[a b c]",
// because these are read by people: the In rule lists the values a field may
// take, and Go's slice syntax in the middle of a sentence is a defect.
func displayValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []string:
		return strings.Join(v, ", ")
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}
