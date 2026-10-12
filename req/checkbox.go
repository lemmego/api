package req

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ggicci/httpin/core"
)

// checkbox is a bool that understands what an HTML form sends.
//
// A checked box sends `on`; some clients send `yes`. strconv.ParseBool knows
// neither, so httpin refused a form that any browser produces — a plain
// <input type="checkbox" name="ama"> arrived as
// `strconv.ParseBool: parsing "on": invalid syntax` and took the whole
// request down with a 500.
//
// DecodeForm has handled these since it was written, which is the real
// argument for this file: two binders in one framework that disagree about
// what `on` means is a bug waiting on whichever path a given struct happens
// to take, and which path that is turns on a struct tag.
//
// An unchecked box sends nothing at all, so absence is handled by httpin
// leaving the field alone rather than by anything here.
type checkbox bool

func (c checkbox) ToString() (string, error) {
	if c {
		return "true", nil
	}
	return "false", nil
}

func (c *checkbox) FromString(raw string) error {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "yes":
		*c = true
		return nil
	case "", "off", "no":
		*c = false
		return nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("%q is not a true/false value", raw)
	}
	*c = checkbox(parsed)
	return nil
}

// Registered for bool globally rather than under a name, because the
// alternative is every bool field in every project carrying `in:"coder=..."`
// to be able to receive a checkbox — and the one that forgets fails at
// runtime, on a form that looks perfectly ordinary.
func init() {
	core.RegisterCoder[bool](func(b *bool) (core.Stringable, error) {
		return (*checkbox)(b), nil
	})
}
