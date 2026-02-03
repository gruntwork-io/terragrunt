package models

import (
	"net/url"
	"path"
	"strings"
)

func resolveRelativeReference(base *url.URL, link string) string {
	if link == "" {
		return link
	}

	if strings.Contains(link, "://") {
		return link
	}

	if strings.HasPrefix(link, "/") {
		return (&url.URL{
			Scheme: base.Scheme,
			Host:   base.Host,
			Path:   link,
		}).String()
	}

	return base.ResolveReference(
		&url.URL{
			Path: path.Join(
				base.Path,
				link,
			),
		},
	).String()
}
