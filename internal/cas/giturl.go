package cas

import "net/url"

// GitURLParams holds the go-getter query parameters [StripGitURLParams] lifts
// out of a git URL.
type GitURLParams struct {
	Ref    string
	SSHKey string
}

// StripGitURLParams returns a copy of u with the go-getter ref, depth and sshkey
// query parameters removed, along with the ref and sshkey. None is a native git
// URL parameter, so they must not survive into the URL handed to git. Git would
// treat a trailing "?depth=1" as part of the repository name and reject the
// clone, and an sshkey would land on git's command line.
//
// ref selects the revision to check out, and sshkey is the base64 private key
// git connects with (see [WithSSHKey]). depth is discarded rather than honored.
// Clone depth comes solely from --cas-clone-depth, whose default of 1 applies
// whether or not the flag was passed.
func StripGitURLParams(u *url.URL) (*url.URL, GitURLParams) {
	stripped := u.Clone()

	q := stripped.Query()
	if len(q) == 0 {
		return stripped, GitURLParams{}
	}

	params := GitURLParams{Ref: q.Get("ref"), SSHKey: q.Get("sshkey")}

	q.Del("ref")
	q.Del("depth")
	q.Del("sshkey")

	stripped.RawQuery = q.Encode()

	return stripped, params
}
