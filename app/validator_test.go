package app

import (
	"testing"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if !v.IsValid() {
		t.Error("new validator should be valid")
	}
	if err := v.Validate(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestAddError(t *testing.T) {
	v := NewValidator()
	v.AddError("email", "is required")
	if v.IsValid() {
		t.Error("validator should be invalid after adding error")
	}
	json := v.ErrorsJSON()
	if _, ok := json["email"]; !ok {
		t.Error("expected email field in errors JSON")
	}
}

func TestRequired(t *testing.T) {
	v := NewValidator()
	v.Field("name", "").Required()
	if v.IsValid() {
		t.Error("required field should fail when empty")
	}

	v2 := NewValidator()
	v2.Field("name", "john").Required()
	if !v2.IsValid() {
		t.Error("required field should pass when non-empty")
	}
}

func TestEmail(t *testing.T) {
	v := NewValidator()
	v.Field("email", "invalid").Email()
	if v.IsValid() {
		t.Error("invalid email should fail")
	}

	v2 := NewValidator()
	v2.Field("email", "user@example.com").Email()
	if !v2.IsValid() {
		t.Error("valid email should pass")
	}
}

func TestMin(t *testing.T) {
	v := NewValidator()
	v.Field("num", 2).Min(3)
	if v.IsValid() {
		t.Error("small number should fail Min(3)")
	}
	v2 := NewValidator()
	v2.Field("num", 5).Min(3)
	if !v2.IsValid() {
		t.Error("large enough number should pass Min(3)")
	}
}

func TestMax(t *testing.T) {
	v := NewValidator()
	v.Field("num", 10).Max(5)
	if v.IsValid() {
		t.Error("too large number should fail Max(5)")
	}
	v2 := NewValidator()
	v2.Field("num", 3).Max(5)
	if !v2.IsValid() {
		t.Error("number within range should pass Max(5)")
	}
}

func TestBetween(t *testing.T) {
	v := NewValidator()
	v.Field("age", 1).Between(18, 65)
	if v.IsValid() {
		t.Error("value below range should fail")
	}

	v2 := NewValidator()
	v2.Field("age", 30).Between(18, 65)
	if !v2.IsValid() {
		t.Error("value in range should pass")
	}
}

func TestURL(t *testing.T) {
	v := NewValidator()
	v.Field("url", "not-a-url").URL()
	if v.IsValid() {
		t.Error("invalid URL should fail")
	}

	v2 := NewValidator()
	v2.Field("url", "https://example.com").URL()
	if !v2.IsValid() {
		t.Error("valid URL should pass")
	}
}

func TestIP(t *testing.T) {
	v := NewValidator()
	v.Field("ip", "not-an-ip").IP()
	if v.IsValid() {
		t.Error("invalid IP should fail")
	}

	v2 := NewValidator()
	v2.Field("ip", "192.168.1.1").IP()
	if !v2.IsValid() {
		t.Error("valid IP should pass")
	}
}

func TestUUID(t *testing.T) {
	v := NewValidator()
	v.Field("uuid", "not-a-uuid").UUID()
	if v.IsValid() {
		t.Error("invalid UUID should fail")
	}

	v2 := NewValidator()
	v2.Field("uuid", "550e8400-e29b-41d4-a716-446655440000").UUID()
	if !v2.IsValid() {
		t.Error("valid UUID should pass")
	}
}

func TestNumeric(t *testing.T) {
	v := NewValidator()
	v.Field("num", "abc").Numeric()
	if v.IsValid() {
		t.Error("non-numeric should fail")
	}

	v2 := NewValidator()
	v2.Field("num", "42").Numeric()
	if !v2.IsValid() {
		t.Error("numeric string should pass")
	}
}

func TestAlpha(t *testing.T) {
	v := NewValidator()
	v.Field("name", "abc123").Alpha()
	if v.IsValid() {
		t.Error("alphanumeric should fail Alpha")
	}

	v2 := NewValidator()
	v2.Field("name", "john").Alpha()
	if !v2.IsValid() {
		t.Error("alpha string should pass")
	}
}

func TestRegex(t *testing.T) {
	v := NewValidator()
	v.Field("code", "abc").Regex("^[0-9]+$")
	if v.IsValid() {
		t.Error("non-matching regex should fail")
	}

	v2 := NewValidator()
	v2.Field("code", "123").Regex("^[0-9]+$")
	if !v2.IsValid() {
		t.Error("matching regex should pass")
	}
}

func TestJSONRule(t *testing.T) {
	v := NewValidator()
	v.Field("data", "{invalid json}").JSON()
	if v.IsValid() {
		t.Error("invalid JSON should fail")
	}

	v2 := NewValidator()
	v2.Field("data", `{"key":"value"}`).JSON()
	if !v2.IsValid() {
		t.Error("valid JSON should pass")
	}
}

func TestCustom(t *testing.T) {
	v := NewValidator()
	v.Field("x", "value").Custom(func(val any) (bool, string) {
		return false, "always fails"
	})
	if v.IsValid() {
		t.Error("custom validator that returns false should fail")
	}
}

func TestErrorsJSONFormat(t *testing.T) {
	v := NewValidator()
	v.AddError("field1", "error 1")
	v.AddError("field1", "error 2")
	v.AddError("field2", "error 3")

	json := v.ErrorsJSON()
	if len(json) != 2 {
		t.Errorf("expected 2 fields in errors JSON, got %d", len(json))
	}
	if len(json["field1"]) != 2 {
		t.Errorf("expected 2 errors for field1, got %d", len(json["field1"]))
	}
}

func TestForEach(t *testing.T) {
	v := NewValidator()
	items := []string{"good", "", "good"}
	v.Field("items", items).ForEach(func(f *vField) *vField { return f.Required() })
	if v.IsValid() {
		t.Error("ForEach should catch empty item")
	}

	v2 := NewValidator()
	v2.Field("items", []string{"a", "b", "c"}).ForEach(func(f *vField) *vField { return f.Required() })
	if !v2.IsValid() {
		t.Error("all non-empty items should pass ForEach")
	}
}
