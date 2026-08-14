package cas

import "net/url"

// StripGitURLParams removes the go-getter query parameters CAS consumes (ref,
// depth) from u and returns the ref. Neither is a native git URL parameter, so
// they must not survive into the URL handed to git: git would treat a trailing
// "?depth=1" as part of the repository name and reject the clone.
//
// ref selects the revision to check out. depth is dropped rather than honored:
// clone depth is a CLI concern (--cas-clone-depth), and CLI arguments take
// precedence over configuration throughout Terragrunt, so a depth on a
// configured source URL never overrides the ambient depth.
//
// u is mutated: its RawQuery is rewritten with both parameters removed.
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
