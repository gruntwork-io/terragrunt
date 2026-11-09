package models

import (
	"net/url"
	"path"
	"strings"
)

// ResolveRelativeReference resolves link against base as a directory, keeping its escaped path, query and fragment.
func ResolveRelativeReference(base *url.URL, link string) (*url.URL, error) {
	ref, err := url.Parse(link)
	if err != nil || ref.IsAbs() {
		return ref, err
	}

	// Treat base as a directory for a relative path.
	if ref.Host == "" && ref.Path != "" && !strings.HasPrefix(ref.Path, "/") && !strings.HasSuffix(base.Path, "/") {
		dir := *base

		dir.Path += "/"
		if dir.RawPath != "" {
			dir.RawPath += "/"
		}

		base = &dir
	}

	return base.ResolveReference(ref), nil
}

// resolveLink resolves link against base, leaving an empty or unparsable link as it is.
func resolveLink(base *url.URL, link string) string {
	if link == "" {
		return link
	}

	resolved, err := ResolveRelativeReference(base, link)
	if err != nil {
		return link
	}

	return resolved.String()
}

// FilenameFromURL extracts a clean filename from a URL string, stripping query parameters and fragments.
func FilenameFromURL(rawURL string) string {
	name := rawURL
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
		name = parsed.Path
	}

	// Filesystem mirror paths are OS-native, so Windows separators reach this too.
	return path.Base(strings.ReplaceAll(name, `\`, "/"))
}
