package models

import (
	"net/url"
	"path"
	"strings"
)

func resolveRelativeReference(base *url.URL, link string) string {
	if link != "" && !strings.HasPrefix(link, base.Scheme) {
		link = base.ResolveReference(&url.URL{Path: path.Join(base.Path, link)}).String()
	}

	return link
}
