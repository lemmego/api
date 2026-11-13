package app

import (
	"strings"
	"testing"
)

// errorFor runs one rule and returns the message, or "" when it passed.
func errorFor(t *testing.T, apply func(*Validator)) string {
	t.Helper()
	v := NewValidator()
	apply(v)
	if v.Validate() == nil {
		return ""
	}
	for _, messages := range v.Errors {
		if len(messages) > 0 {
			return messages[0]
		}
	}
	t.Fatal("Validate reported a failure with no message behind it")
	return ""
}

// TestTheSizeRulesEnforceAStringsLength is the bug these rules shipped with.
//
// All three opened with `if v, ok := f.value.(int); ok`, so on a string they
// silently did nothing — which meant every scaffolded project's
// Min(8) on a password enforced no minimum, and a Max(255) on a name enforced
// no maximum, with nothing anywhere to report it.
func TestTheSizeRulesEnforceAStringsLength(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*Validator)
		want  string
	}{
		{"a short password is refused",
			func(v *Validator) { v.Field("password", "abc").Min(8) },
			"This field must be at least 8 characters"},
		{"an eight-character password is accepted",
			func(v *Validator) { v.Field("password", "abcdefgh").Min(8) }, ""},
		{"an over-long name is refused",
			func(v *Validator) { v.Field("name", strings.Repeat("x", 256)).Max(255) },
			"This field must not exceed 255 characters"},
		{"a name at the limit is accepted",
			func(v *Validator) { v.Field("name", strings.Repeat("x", 255)).Max(255) }, ""},
		{"a string outside the range is refused",
			func(v *Validator) { v.Field("handle", "ab").Between(3, 20) },
			"This field must be between 3 and 20 characters"},
		{"a string inside the range is accepted",
			func(v *Validator) { v.Field("handle", "ada").Between(3, 20) }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorFor(t, tc.apply); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAStringIsCountedInCharactersNotBytes, and this is the case that makes
// the choice security-relevant rather than cosmetic.
//
// A byte count is larger than a rune count for any non-ASCII text, so
// byte-counting is the *permissive* direction for a minimum: "密码密码" is four
// characters and twelve bytes, so it would satisfy Min(8) while being half
// the length the rule asks for.
func TestAStringIsCountedInCharactersNotBytes(t *testing.T) {
	short := "密码密码" // 4 characters, 12 bytes
	if len(short) <= 8 {
		t.Fatalf("the fixture is wrong: %q is %d bytes, so it does not "+
			"distinguish the two counts", short, len(short))
	}

	got := errorFor(t, func(v *Validator) { v.Field("password", short).Min(8) })
	if got == "" {
		t.Error("a four-character password satisfied Min(8), so the rule is " +
			"counting bytes — which is the permissive direction for a minimum")
	}

	// And the other direction: a maximum must not punish non-ASCII text for
	// being wider in bytes than it is in characters.
	long := strings.Repeat("é", 200) // 200 characters, 400 bytes
	if got := errorFor(t, func(v *Validator) { v.Field("bio", long).Max(255) }); got != "" {
		t.Errorf("200 accented characters failed Max(255): %q", got)
	}
}

// TestTheSizeRulesMeasureCollections, whose message variants have existed in
// the catalogue since it was written and were emitted by nothing.
func TestTheSizeRulesMeasureCollections(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*Validator)
		want  string
	}{
		{"too few items",
			func(v *Validator) { v.Field("topics", []string{"go"}).Min(2) },
			"This field must have at least 2 items"},
		{"too many items",
			func(v *Validator) { v.Field("topics", []int{1, 2, 3}).Max(2) },
			"This field must not have more than 2 items"},
		{"outside the range",
			func(v *Validator) { v.Field("topics", []any{1, 2, 3, 4}).Between(1, 3) },
			"This field must have between 1 and 3 items"},
		{"a map is counted by entries",
			func(v *Validator) { v.Field("meta", map[string]string{"a": "1"}).Min(2) },
			"This field must have at least 2 items"},
		{"enough items",
			func(v *Validator) { v.Field("topics", []string{"go", "sql"}).Min(2) }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorFor(t, tc.apply); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEveryNumericKindIsMeasured: an int-only assertion was a second silent
// no-op in the same three rules, and a JSON body binds a number to float64.
func TestEveryNumericKindIsMeasured(t *testing.T) {
	for _, value := range []any{
		3, int8(3), int16(3), int32(3), int64(3),
		uint(3), uint8(3), uint16(3), uint32(3), uint64(3),
		float32(3), float64(3),
	} {
		got := errorFor(t, func(v *Validator) { v.Field("age", value).Min(10) })
		if got != "This field must be at least 10" {
			t.Errorf("%T(3) with Min(10) gave %q", value, got)
		}
	}

	// A fractional value is compared as written rather than truncated.
	if got := errorFor(t, func(v *Validator) { v.Field("rate", 9.5).Min(10) }); got == "" {
		t.Error("9.5 satisfied Min(10)")
	}
	if got := errorFor(t, func(v *Validator) { v.Field("rate", 10.5).Min(10) }); got != "" {
		t.Errorf("10.5 failed Min(10): %q", got)
	}
}

// TestAPointerIsFollowed, because an optional input field is usually a
// *string and a rule applied to one silently did nothing.
func TestAPointerIsFollowed(t *testing.T) {
	short := "abc"
	if got := errorFor(t, func(v *Validator) { v.Field("password", &short).Min(8) }); got == "" {
		t.Error("Min(8) through a *string did nothing")
	}

	// A nil pointer is absent, which is Required's business and not this
	// rule's — so it must not invent a length-zero failure.
	var absent *string
	if got := errorFor(t, func(v *Validator) { v.Field("nickname", absent).Min(8) }); got != "" {
		t.Errorf("a nil pointer failed a size rule: %q", got)
	}
}

// TestAnUnmeasurableValueIsNotAnError, and specifically does not panic.
//
// For a field typed `any` the dynamic type comes from the request, so a panic
// here would be a 500 a client could choose to cause.
func TestAnUnmeasurableValueIsNotAnError(t *testing.T) {
	for _, value := range []any{true, nil, struct{ A int }{1}, make(chan int)} {
		func() {
			defer func() {
				if problem := recover(); problem != nil {
					t.Errorf("%T panicked: %v", value, problem)
				}
			}()
			if got := errorFor(t, func(v *Validator) { v.Field("x", value).Min(8) }); got != "" {
				t.Errorf("%T produced %q", value, got)
			}
		}()
	}
}

// TestTheNumericMessageIsUnchangedForNumbers, so the one case that did work
// keeps saying exactly what it said.
func TestTheNumericMessageIsUnchangedForNumbers(t *testing.T) {
	for _, tc := range []struct {
		apply func(*Validator)
		want  string
	}{
		{func(v *Validator) { v.Field("a", 3).Min(10) }, "This field must be at least 10"},
		{func(v *Validator) { v.Field("a", 30).Max(10) }, "This field must not exceed 10"},
		{func(v *Validator) { v.Field("a", 30).Between(1, 10) }, "This field must be between 1 and 10"},
	} {
		if got := errorFor(t, tc.apply); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}
