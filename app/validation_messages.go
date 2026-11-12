package app

// builtinMessages is the English the framework falls back to when no
// translator is registered.
//
// This is the floor that makes translation a non-breaking change. A unit test
// calling NewValidator(), or a project that never adopts i18n, gets these —
// and every string here is byte-identical to the literal it replaced, so the
// existing suites, screenshots and documentation did not change when the
// mechanism did.
//
// It is a Go map rather than an embedded JSON file so that api/app gains no
// import at all: see translator.go for why this module does not depend on
// lemmego/i18n. The cost is that the same English exists twice, in this map
// and in i18n/catalog/en/. A parity test in i18n/provider — the one package
// that imports both — fails if they drift.
//
// Deliberately **without** :attribute, where Laravel's equivalents all use
// it. Two reasons. Keeping today's strings means zero diff in what anybody
// has already seen; and it is the better copy for how this framework renders
// errors, since both api/res and lemmego/inertia key them by field name and
// show them beside the input, where "This field" reads correctly and "The
// email address field" is redundant. Laravel's :attribute voice exists for a
// summary list at the top of a form.
//
// The parameter is bound on every message regardless, so a project that wants
// the Laravel voice publishes its own validation.json and writes "The
// :attribute field is required." with no code change.
var builtinMessages = map[string]string{
	"validation.required": "This field is required",
	"validation.filled":   "This field must be filled",
	"validation.equals":   "This field must match with the provided value",

	"validation.min.numeric":     "This field must be at least :min",
	"validation.max.numeric":     "This field must not exceed :max",
	"validation.between.numeric": "This field must be between :min and :max",

	// Reserved and shipped before anything emits them.
	//
	// Min, Max and Between type-assert .(int) and so silently do nothing to a
	// string, a slice or a map — which is a real defect, recorded separately.
	// When it is fixed these keys go live, and shipping them now means that
	// fix is a validator-only change with no catalogue migration behind it.
	"validation.min.string":     "This field must be at least :min characters",
	"validation.max.string":     "This field must not exceed :max characters",
	"validation.between.string": "This field must be between :min and :max characters",
	"validation.min.array":      "This field must have at least :min items",
	"validation.max.array":      "This field must not have more than :max items",
	"validation.between.array":  "This field must have between :min and :max items",

	"validation.min.file": "This file is smaller than :min",
	"validation.max.file": "This file is larger than :max",

	"validation.email":       "This field must be a valid email address",
	"validation.url":         "This field must be a valid URL",
	"validation.ip":          "This field must be a valid IP address",
	"validation.uuid":        "This field must be a valid UUID",
	"validation.ulid":        "This field must be a valid ULID",
	"validation.mac_address": "This field must be a valid MAC address",
	"validation.hex_color":   "This field must be a valid hexadecimal color code",
	"validation.json":        "This field must be a valid JSON string",
	"validation.boolean":     "This field must be a boolean value",
	"validation.timezone":    "Invalid timezone",

	"validation.alpha":      "This field must contain only alphabetic characters",
	"validation.numeric":    "This field must contain only numeric characters",
	"validation.alpha_num":  "This field must contain only alphanumeric characters",
	"validation.alpha_dash": "This field may only contain alpha-numeric characters, dashes, and underscores",
	"validation.ascii":      "This field may only contain ASCII characters",

	"validation.date_format": "This field must be a valid date in the format :format",
	"validation.after":       "This field must be a date after :date",
	"validation.before":      "This field must be a date before :date",

	"validation.in":                    "This field must be one of the following: :values",
	"validation.starts_with":           "This field must start with :value",
	"validation.ends_with":             "This field must end with :value",
	"validation.contains":              "This field must contain :value",
	"validation.regex":                 "This field must match the pattern: :pattern",
	"validation.regex_pattern_invalid": "Invalid regular expression pattern",

	"validation.array":       "This field must be an array or slice",
	"validation.array_empty": "This field cannot be empty",
	"validation.distinct":    "This field must contain only unique values",

	"validation.active_url":           "The URL is not active or reachable",
	"validation.active_url_status":    "The URL returned a non-OK status",
	"validation.active_url_too_large": "The URL response is too large",

	"validation.file_unopenable":  "Unable to open the file",
	"validation.file_undecodable": "Unable to decode the image",
	"validation.file_unreadable":  "Unable to read the file",
	"validation.file_type":        "File type must be one of: :values",
	"validation.dimensions":       "Image dimensions must be :widthx:height",

	"validation.upload_unreadable":    "This file could not be read",
	"validation.mimes":                "This file must be one of: :values",
	"validation.image":                "This file is not an image",
	"validation.image_dimensions.min": "This image must be at least :widthx:height pixels",
	"validation.image_dimensions.max": "This image must be at most :widthx:height pixels",
	"validation.file_prefixed":        ":file — :message",

	"validation.check_failed": "This field could not be validated",
	"validation.uncheckable":  "This value could not be checked",
	"validation.unique":       "This :attribute is already taken",
	"validation.exists":       "This :attribute does not exist",

	// The eight standard HTTP messages, keyed by status.
	//
	// The sentinels in errors.go are package-level singletons shared across
	// goroutines and compared by identity in app.go, so a localised string
	// can never be written into one. They are looked up here instead, at the
	// single point where a sentinel's message is rendered.
	"http.400": "Bad Request",
	"http.401": "Unauthorized",
	"http.403": "Forbidden",
	"http.404": "Not Found",
	"http.405": "Method Not Allowed",
	"http.419": "Page Expired",
	"http.422": "Unprocessable Entity",
	"http.500": "Internal Server Error",
}
