package cas

import "net/url"

// StripGitURLParams returns a copy of u with the go-getter ref and depth query
// parameters removed, along with the ref. Neither is a native git URL
// parameter, so they must not survive into the URL handed to git. Git would
// treat a trailing "?depth=1" as part of the repository name and reject the
// clone.
//
// ref selects the revision to check out. depth is discarded rather than
// honored. Clone depth comes solely from --cas-clone-depth, whose default of 1
// applies whether or not the flag was passed.
func StripGitURLParams(u *url.URL) (*url.URL, string) {
	stripped := u.Clone()

	q := stripped.Query()
	if len(q) == 0 {
		return stripped, ""
	}

	ref := q.Get("ref")

	q.Del("ref")
	q.Del("depth")

	stripped.RawQuery = q.Encode()

	return stripped, ref
}

// RedactURL returns rawURL with any userinfo removed, so a token embedded
// in an https URL never reaches an error or a log line. Input net/url
// cannot parse (SCP form, which carries no secret) comes back unchanged.
func RedactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}

	u.User = nil

	return u.String()
}
