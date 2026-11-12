package app

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// fakeTranslator is a catalogue of exactly what a test hands it. The real one
// lives in lemmego/i18n, which this module deliberately does not import.
type fakeTranslator struct {
	messages map[string]string
	asked    []string
}

func (f *fakeTranslator) Get(key string, repl ...map[string]any) string {
	f.asked = append(f.asked, key)
	text, ok := f.messages[key]
	if !ok {
		return key
	}
	var values map[string]any
	if len(repl) > 0 {
		values = repl[0]
	}
	return Interpolate(text, values)
}

func (f *fakeTranslator) Has(key string) bool {
	_, ok := f.messages[key]
	return ok
}

// TestTheEnglishIsUnchanged pins every message this change rewrote.
//
// It exists because the suite it was supposed to be checked against does not
// check it: validator_test.go asserts that rules fire and never once asserts
// what they say. So "the existing tests still pass" proved the rules still
// fire and nothing at all about the words, which is the vacuous-checkpoint
// failure this project keeps finding. These are the strings that were inline
// literals before there was a catalogue, byte for byte.
func TestTheEnglishIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule func(*Validator)
		want string
	}{
		{"required", func(v *Validator) { v.Field("a", "").Required() },
			"This field is required"},
		{"email", func(v *Validator) { v.Field("a", "nope").Email() },
			"This field must be a valid email address"},
		{"min", func(v *Validator) { v.Field("a", 3).Min(10) },
			"This field must be at least 10"},
		{"max", func(v *Validator) { v.Field("a", 30).Max(10) },
			"This field must not exceed 10"},
		{"between", func(v *Validator) { v.Field("a", 30).Between(1, 10) },
			"This field must be between 1 and 10"},
		{"in", func(v *Validator) { v.Field("a", "x").In([]string{"red", "green"}) },
			"This field must be one of the following: red, green"},
		{"starts_with", func(v *Validator) { v.Field("a", "x").StartsWith("ab") },
			"This field must start with ab"},
		{"url", func(v *Validator) { v.Field("a", "x").URL() },
			"This field must be a valid URL"},
		{"uuid", func(v *Validator) { v.Field("a", "x").UUID() },
			"This field must be a valid UUID"},
		{"alpha_dash", func(v *Validator) { v.Field("a", "x y").AlphaDash() },
			"This field may only contain alpha-numeric characters, dashes, and underscores"},
		{"timezone", func(v *Validator) { v.Field("a", "Mars/Olympus").Timezone() },
			"Invalid timezone"},
		{"json", func(v *Validator) { v.Field("a", "{").JSON() },
			"This field must be a valid JSON string"},
		{"regex", func(v *Validator) { v.Field("a", "x").Regex("^[0-9]+$") },
			"This field must match the pattern: ^[0-9]+$"},
		{"date_format", func(v *Validator) { v.Field("a", "nope").Date("2006-01-02") },
			"This field must be a valid date in the format 2006-01-02"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := NewValidator()
			tc.rule(v)
			got := v.Errors["a"]
			if len(got) != 1 {
				t.Fatalf("recorded %d errors, want 1: %v", len(got), got)
			}
			if got[0] != tc.want {
				t.Errorf("message = %q\n          want %q", got[0], tc.want)
			}
		})
	}
}

// TestEveryKeyARuleUsesExists parses validator.go and checks each key passed
// to fail or failUpload against the built-in catalogue.
//
// A typo'd key is invisible otherwise: it renders as the raw key, in
// production, in whichever rule nobody happens to trip in a test. The
// compiler cannot catch it because a key is a string, so this does.
func TestEveryKeyARuleUsesExists(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "validator.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing validator.go: %v", err)
	}

	var found int
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		var arg ast.Expr
		switch selector.Sel.Name {
		case "fail":
			if len(call.Args) > 0 {
				arg = call.Args[0]
			}
		case "failUpload":
			if len(call.Args) > 2 {
				arg = call.Args[2]
			}
		default:
			return true
		}

		literal, ok := arg.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			t.Errorf("%s: the rule name passed to %s is not a literal, so it cannot be "+
				"checked here", fset.Position(call.Pos()), selector.Sel.Name)
			return true
		}

		found++
		key := "validation." + strings.Trim(literal.Value, `"`)
		if _, ok := builtinMessages[key]; !ok {
			t.Errorf("%s: the rule uses the key %q, which is not in builtinMessages, so "+
				"it would render as the raw key", fset.Position(call.Pos()), key)
		}
		return true
	})

	// The conversion rewrote 63 call sites plus Check's fallback. A parser
	// that silently matched nothing would make this pass having checked
	// nothing at all.
	if found < 50 {
		t.Fatalf("only %d rule keys were found in validator.go, so this test is not "+
			"reading what it thinks it is", found)
	}
}

func TestATranslatorOverridesTheBuiltinEnglish(t *testing.T) {
	fake := &fakeTranslator{messages: map[string]string{
		"validation.required": "Ce champ est obligatoire",
	}}
	v := NewValidator(WithTranslator(fake))
	v.Field("email", "").Required()

	if got := v.Errors["email"][0]; got != "Ce champ est obligatoire" {
		t.Errorf("message = %q, want the translation", got)
	}
}

func TestAnUntranslatedKeyFallsBackToTheBuiltinEnglish(t *testing.T) {
	// A catalogue a translator has only partly filled in: the rule that fires
	// is not in it.
	fake := &fakeTranslator{messages: map[string]string{"validation.email": "..."}}
	v := NewValidator(WithTranslator(fake))
	v.Field("a", "").Required()

	if got := v.Errors["a"][0]; got != "This field is required" {
		t.Errorf("message = %q; an untranslated rule must still read as a sentence", got)
	}
}

func TestParametersReachTheTranslation(t *testing.T) {
	fake := &fakeTranslator{messages: map[string]string{
		"validation.between.numeric": "entre :min et :max",
	}}
	v := NewValidator(WithTranslator(fake))
	v.Field("a", 99).Between(1, 10)

	if got := v.Errors["a"][0]; got != "entre 1 et 10" {
		t.Errorf("message = %q", got)
	}
}

func TestAttributeResolvesInOrder(t *testing.T) {
	t.Run("the per-validator label wins", func(t *testing.T) {
		fake := &fakeTranslator{messages: map[string]string{
			"validation.required":              "The :attribute is needed",
			"validation.attributes.email_addr": "From the catalogue",
		}}
		v := NewValidator(WithTranslator(fake)).
			Attributes(map[string]string{"email_addr": "Your email"})
		v.Field("email_addr", "").Required()

		if got := v.Errors["email_addr"][0]; got != "The Your email is needed" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("then the catalogue", func(t *testing.T) {
		fake := &fakeTranslator{messages: map[string]string{
			"validation.required":              "The :attribute is needed",
			"validation.attributes.email_addr": "Email address",
		}}
		v := NewValidator(WithTranslator(fake))
		v.Field("email_addr", "").Required()

		if got := v.Errors["email_addr"][0]; got != "The Email address is needed" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("then the field name, made readable", func(t *testing.T) {
		fake := &fakeTranslator{messages: map[string]string{
			"validation.required": "The :attribute is needed",
		}}
		v := NewValidator(WithTranslator(fake))
		v.Field("email_addr", "").Required()

		if got := v.Errors["email_addr"][0]; got != "The email addr is needed" {
			t.Errorf("message = %q; the last resort must never be empty, or the "+
				"message renders with a hole in it", got)
		}
	})

	t.Run("an indexed field collapses to a wildcard", func(t *testing.T) {
		// A field inside ForEach is named "tags.0", so no catalogue could
		// list every index.
		fake := &fakeTranslator{messages: map[string]string{
			"validation.required":          "The :attribute is needed",
			"validation.attributes.tags.*": "Tag",
		}}
		v := NewValidator(WithTranslator(fake))
		v.Field("tags.0", "").Required()

		if got := v.Errors["tags.0"][0]; got != "The Tag is needed" {
			t.Errorf("message = %q", got)
		}
	})
}

// TestCustomAndCheckStillTakeEnglish is the compatibility guarantee: a rule
// written before there was a catalogue keeps working unchanged.
func TestCustomAndCheckStillTakeEnglish(t *testing.T) {
	v := NewValidator()
	v.Field("handle", "settings").Custom(func(any) (bool, string) {
		return false, "That handle is reserved"
	})
	if got := v.Errors["handle"][0]; got != "That handle is reserved" {
		t.Errorf("Custom's message = %q", got)
	}

	v2 := NewValidator()
	v2.Field("email", "a@b.c").Check(func(any) (bool, string, error) {
		return false, "that email is taken", nil
	})
	if got := v2.Errors["email"][0]; got != "that email is taken" {
		t.Errorf("Check's message = %q", got)
	}
}

func TestCustomCanReturnAKeyInstead(t *testing.T) {
	fake := &fakeTranslator{messages: map[string]string{
		"validation.custom.handle_reserved": "Ce pseudo est réservé",
	}}
	v := NewValidator(WithTranslator(fake))
	v.Field("handle", "settings").Custom(func(any) (bool, string) {
		return false, "validation.custom.handle_reserved"
	})
	if got := v.Errors["handle"][0]; got != "Ce pseudo est réservé" {
		t.Errorf("message = %q; a rule should be able to opt in by returning a key", got)
	}
}

func TestCheckReportsAnUnrunnableRuleSeparately(t *testing.T) {
	v := NewValidator()
	boom := errors.New("the database is down")
	v.Field("email", "a@b.c").Check(func(any) (bool, string, error) {
		return false, "", boom
	})

	if got := v.Errors["email"][0]; got != "This field could not be validated" {
		t.Errorf("message = %q", got)
	}
	if !errors.Is(v.Err(), boom) {
		t.Errorf("Err() = %v, want the underlying failure so a handler can answer 503", v.Err())
	}
}

// TestInterpolationRules pins the two properties the substituter exists for,
// and which the browser half has to match.
func TestInterpolationRules(t *testing.T) {
	t.Run("a longer placeholder wins over a shorter prefix", func(t *testing.T) {
		got := Interpolate("one of :values, not :value", map[string]any{
			"value":  "red",
			"values": "green, blue",
		})
		if want := "one of green, blue, not red"; got != want {
			t.Errorf("got %q, want %q\nthe pairs are not sorted longest-first", got, want)
		}
	})

	t.Run("a replacement containing a colon is not reinterpreted", func(t *testing.T) {
		got := Interpolate(":file — :message", map[string]any{
			"file":    ":message.txt",
			"message": "This file is not an image",
		})
		if want := ":message.txt — This file is not an image"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("an unknown placeholder is left verbatim", func(t *testing.T) {
		if got := Interpolate("Hello, :nmae", map[string]any{"name": "Ada"}); got != "Hello, :nmae" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("adjacent placeholders render", func(t *testing.T) {
		got := Interpolate("must be :widthx:height", map[string]any{"width": 100, "height": 50})
		if want := "must be 100x50"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("a string slice reads as a sentence", func(t *testing.T) {
		got := Interpolate("one of :values", map[string]any{"values": []string{"a", "b"}})
		if want := "one of a, b"; got != want {
			t.Errorf("got %q, want %q; Go's slice syntax in the middle of a sentence "+
				"is a defect", got, want)
		}
	})
}

func TestTheBuiltinCatalogueIsCopiedNotShared(t *testing.T) {
	first := BuiltinValidationMessages()
	first["validation.required"] = "tampered"
	if BuiltinValidationMessages()["validation.required"] != "This field is required" {
		t.Error("BuiltinValidationMessages handed out the real map, so one caller can " +
			"change every validator in the process")
	}
}
