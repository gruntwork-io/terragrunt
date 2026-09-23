// Package redact holds values that may carry credentials, rendering them
// without the credentials wherever they are formatted.
package redact

import (
	"net/url"
	"strings"
)

// Placeholder replaces each query value when a [URL] is rendered.
const Placeholder = "REDACTED"

// forcedGetterSeparator divides a forced getter name, such as git or s3, from
// the address it applies to.
const forcedGetterSeparator = "::"

// revisionKeys are the query keys that name which revision of a source an
// address points at: the ref of a git source, the version of a registry
// source. They select code rather than authorize a fetch, so rendering them
// as written reveals no credential.
var revisionKeys = map[string]struct{}{
	"ref":     {},
	"version": {},
}

// URL is an address that may carry credentials in its userinfo or its query.
// Formatting one renders [URL.String], so the full address leaves only
// through [URL.Reveal].
//
// fmt does not call String on an unexported struct field when it prints the
// struct holding it, so a URL kept in an unexported field of a struct
// formatted with %v prints its raw address.
type URL struct {
	raw string
}

// NewURL wraps raw.
func NewURL(raw string) URL {
	return URL{raw: raw}
}

// String renders the address with its user and password removed and every
// query value but the revision replaced by [Placeholder]. A forced getter
// prefix such as git:: is kept, and the address after it is rendered the same
// way. An address [url.Parse] rejects, such as the SCP form git@host:path, is
// returned unchanged.
//
// The ref and version query values render as written, so a rendered address
// still names the revision it points at.
func (u URL) String() string {
	forced, rest, ok := strings.Cut(u.raw, forcedGetterSeparator)
	if ok && isGetterName(forced) {
		return forced + forcedGetterSeparator + render(rest)
	}

	return render(u.raw)
}

// GoString renders the address as [URL.String] does, covering the %#v verb,
// which does not consult String.
func (u URL) GoString() string {
	return u.String()
}

// Reveal returns the address as written, credentials included, for the call
// that has to carry it.
func (u URL) Reveal() string {
	return u.raw
}

// WithoutUserinfo returns the address with its user and password removed and
// everything else, the query included, as written. An address [url.Parse]
// rejects is returned unchanged. It keys records that must stay the same when
// a credential rotates, and is not for display, since the query can still
// carry credentials.
func (u URL) WithoutUserinfo() string {
	parsed, err := url.Parse(u.raw)
	if err != nil || parsed.User == nil {
		return u.raw
	}

	parsed.User = nil

	return parsed.String()
}

// render removes the userinfo from raw and replaces every query value but the
// ones named by [revisionKeys] with [Placeholder], returning raw unchanged
// when [url.Parse] rejects it.
func render(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	parsed.User = nil

	if parsed.RawQuery == "" {
		return parsed.String()
	}

	query := parsed.Query()

	for key, values := range query {
		if _, revision := revisionKeys[key]; revision {
			continue
		}

		for i := range values {
			values[i] = Placeholder
		}
	}

	parsed.RawQuery = query.Encode()

	return parsed.String()
}

// isGetterName reports whether name can be a forced getter name: one or more
// ASCII letters and digits.
func isGetterName(name string) bool {
	return name != "" && !strings.ContainsFunc(name, notASCIIAlphanumeric)
}

// notASCIIAlphanumeric reports whether r is anything but an ASCII letter or
// digit.
func notASCIIAlphanumeric(r rune) bool {
	return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
}
