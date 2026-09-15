package redact_test

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/redact"
)

func TestURLString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "user and password",
			in:   "https://user:token@example.com/org/repo.git",
			want: "https://example.com/org/repo.git",
		},
		{
			name: "token as user",
			in:   "https://ghp_abc123@github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "password only",
			in:   "https://:secret@example.com/org/repo.git",
			want: "https://example.com/org/repo.git",
		},
		{
			name: "ssh user",
			in:   "ssh://git@example.com/org/repo.git",
			want: "ssh://example.com/org/repo.git",
		},
		{
			name: "query values",
			in:   "https://example.com/mod.tgz?token=abc&archive=zip",
			want: "https://example.com/mod.tgz?archive=REDACTED&token=REDACTED",
		},
		{
			name: "repeated query key",
			in:   "https://example.com/mod.tgz?sig=a&sig=b",
			want: "https://example.com/mod.tgz?sig=REDACTED&sig=REDACTED",
		},
		{
			name: "file url unchanged",
			in:   "file:///abs/path/repo.git",
			want: "file:///abs/path/repo.git",
		},
		{
			name: "forced getter with userinfo",
			in:   "git::https://user:token@example.com/org/repo.git//modules/vpc",
			want: "git::https://example.com/org/repo.git//modules/vpc",
		},
		{
			name: "forced getter with credential query",
			in:   "s3::https://s3.amazonaws.com/bucket/key.tgz?aws_access_key_id=AKIAEXAMPLE&aws_access_key_secret=SECRET",
			want: "s3::https://s3.amazonaws.com/bucket/key.tgz?aws_access_key_id=REDACTED&aws_access_key_secret=REDACTED",
		},
		{
			name: "scp form unchanged",
			in:   "git@github.com:org/repo.git",
			want: "git@github.com:org/repo.git",
		},
		{
			name: "unparsable unchanged",
			in:   "https://example.com/%zz",
			want: "https://example.com/%zz",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, redact.NewURL(tt.in).String())
		})
	}
}

func TestURLFormatting(t *testing.T) {
	t.Parallel()

	type holder struct {
		Source redact.URL
	}

	u := redact.NewURL("https://user:token@example.com/repo.git?sig=abc")
	want := "https://example.com/repo.git?sig=REDACTED"

	verbs := []struct {
		verb string
		want string
	}{
		{verb: "%s", want: want},
		{verb: "%v", want: want},
		{verb: "%q", want: strconv.Quote(want)},
		{verb: "%#v", want: want},
	}

	for _, tc := range verbs {
		assert.Equal(t, tc.want, fmt.Sprintf(tc.verb, u), tc.verb)
	}

	for _, verb := range []string{"%v", "%+v", "%#v"} {
		got := fmt.Sprintf(verb, holder{Source: u})

		assert.Contains(t, got, want, verb)
		assert.NotContains(t, got, "token", verb)
		assert.NotContains(t, got, "sig=abc", verb)
	}
}

func TestURLReveal(t *testing.T) {
	t.Parallel()

	raw := "git::https://user:token@example.com/org/repo.git//modules/vpc?ref=v1.0.0"

	assert.Equal(t, raw, redact.NewURL(raw).Reveal())
}

func TestURLWithoutUserinfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "userinfo dropped, query kept",
			in:   "https://x:token@example.com/a.git?version=1",
			want: "https://example.com/a.git?version=1",
		},
		{
			name: "no userinfo returned as written",
			in:   "https://example.com/a.git?b=2&a=1",
			want: "https://example.com/a.git?b=2&a=1",
		},
		{
			name: "scp form unchanged",
			in:   "git@github.com:org/repo.git",
			want: "git@github.com:org/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, redact.NewURL(tt.in).WithoutUserinfo())
		})
	}
}

func FuzzURLString(f *testing.F) {
	for _, seed := range []string{
		"https://user:token@example.com/org/repo.git?sig=abc",
		"https://:secret@example.com/repo.git",
		"git::https://user:token@example.com/org/repo.git",
		"git@github.com:org/repo.git",
		"https://example.com/%zz",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		rendered := redact.NewURL(raw).String()

		parsed, err := url.Parse(raw)
		if err != nil || parsed.User == nil {
			return
		}

		password, ok := parsed.User.Password()
		if !ok || password == "" {
			return
		}

		if strings.Contains(hostAndPath(parsed), password) {
			return
		}

		reparsed, err := url.Parse(rendered)
		if err != nil {
			assert.NotContains(t, rendered, "@"+parsed.Host)

			return
		}

		assert.Nil(t, reparsed.User)
		assert.NotContains(t, hostAndPath(reparsed), password)
	})
}

// hostAndPath renders u without its userinfo and query, the parts of an
// address a password cannot legitimately reappear in after rendering.
func hostAndPath(u *url.URL) string {
	stripped := *u
	stripped.User = nil
	stripped.RawQuery = ""

	return stripped.String()
}
