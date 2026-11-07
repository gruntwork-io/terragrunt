package models

import (
	"net/url"
	"path"
	"strings"
)

// resolveRelativeReference resolves link against base as a directory, keeping query and fragment.
func resolveRelativeReference(base *url.URL, link string) string {
	if link == "" {
		return link
	}

	if strings.Contains(link, "://") {
		return link
	}

	ref, err := url.Parse(link)
	if err != nil {
		return link
	}

	// Join instead of letting ResolveReference replace the base's last segment.
	if ref.Host == "" && !strings.HasPrefix(ref.Path, "/") {
		ref.Path = path.Join(base.Path, ref.Path)
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
