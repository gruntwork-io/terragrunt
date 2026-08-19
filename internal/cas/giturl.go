package cas

import "net/url"

// StripGitURLParams removes the go-getter ref and depth query parameters from u
// and returns the ref. Neither is a native git URL parameter, so they must not
// survive into the URL handed to git: git would treat a trailing "?depth=1" as
// part of the repository name and reject the clone.
//
// ref selects the revision to check out. depth is discarded rather than
// honored: clone depth comes solely from --cas-clone-depth, whose default of 1
// applies whether or not the flag was passed.
//
// The URL is mutated in place, with its RawQuery rewritten to drop both
// parameters.
func StripGitURLParams(u *url.URL) (ref string) {
	q := u.Query()
	if len(q) == 0 {
		return ""
	}

	ref = q.Get("ref")

	q.Del("ref")
	q.Del("depth")

	u.RawQuery = q.Encode()

	return ref
}
