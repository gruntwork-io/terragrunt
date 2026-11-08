package models

import (
	"net/url"
	"path"
	"strings"
)

// ResolveRelativeReference resolves a relative link against base as a directory, keeping its escaped path, query and fragment.
func ResolveRelativeReference(base *url.URL, link string) string {
	ref, err := url.Parse(link)
	if err != nil || link == "" || ref.IsAbs() {
		return link
	}

	// A relative path names an entry under base, which ResolveReference only does when base ends in a slash.
	if ref.Host == "" && ref.Path != "" && !strings.HasPrefix(ref.Path, "/") && !strings.HasSuffix(base.Path, "/") {
		dir := *base

		dir.Path += "/"
		if dir.RawPath != "" {
			dir.RawPath += "/"
		}

		base = &dir
	}

	return base.ResolveReference(ref).String()
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
