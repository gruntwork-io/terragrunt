package spinner_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/stretchr/testify/assert"
)

// TestNewAnimated checks when a reporter can animate.
func TestNewAnimated(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		change func(*spinner.Options)
		name   string
		want   bool
	}{
		{name: "baseline", want: true},
		{name: "no TTY", change: func(o *spinner.Options) { o.OutIsTTY = false }},
		{name: "logs not for humans", change: func(o *spinner.Options) { o.LogsForHumans = false }},
		{name: "nil Out", change: func(o *spinner.Options) { o.Out = nil }},
		{name: "nil Width", change: func(o *spinner.Options) { o.Width = nil }},
		{name: "width 0", change: func(o *spinner.Options) { o.Width = func() int { return 0 } }},
		{name: "width 19", change: func(o *spinner.Options) { o.Width = func() int { return 19 } }},
		{name: "width 20", change: func(o *spinner.Options) { o.Width = func() int { return 20 } }, want: true},
		{name: "empty TERM on linux", change: func(o *spinner.Options) { o.Env["TERM"] = "" }},
		{name: "empty TERM on windows", change: func(o *spinner.Options) {
			o.GOOS = "windows"
			o.Env["TERM"] = ""
		}, want: true},
		{name: "dumb TERM on linux", change: func(o *spinner.Options) { o.Env["TERM"] = "dumb" }},
		{name: "dumb TERM on windows", change: func(o *spinner.Options) {
			o.GOOS = "windows"
			o.Env["TERM"] = "dumb"
		}},
		{name: "uppercase DUMB", change: func(o *spinner.Options) { o.Env["TERM"] = "DUMB" }},
		{name: "CI true", change: func(o *spinner.Options) { o.Env["CI"] = "true" }},
		{name: "CI 1", change: func(o *spinner.Options) { o.Env["CI"] = "1" }},
		{name: "CI false", change: func(o *spinner.Options) { o.Env["CI"] = "false" }, want: true},
		{name: "CI 0", change: func(o *spinner.Options) { o.Env["CI"] = "0" }, want: true},
		{name: "CI empty", change: func(o *spinner.Options) { o.Env["CI"] = "" }, want: true},
		{name: "CI spaced false", change: func(o *spinner.Options) { o.Env["CI"] = " false " }, want: true},
		{name: "TF_BUILD", change: func(o *spinner.Options) { o.Env["TF_BUILD"] = "True" }},
		{name: "TEAMCITY_VERSION", change: func(o *spinner.Options) { o.Env["TEAMCITY_VERSION"] = "1" }},
		{name: "JENKINS_URL", change: func(o *spinner.Options) { o.Env["JENKINS_URL"] = "url" }},
		{name: "TF_IN_AUTOMATION", change: func(o *spinner.Options) { o.Env["TF_IN_AUTOMATION"] = "1" }},
		{name: "darwin", change: func(o *spinner.Options) {
			o.GOOS = "darwin"
			o.Env["TERM"] = "xterm-256color"
		}, want: true},
		{name: "nil Env on linux", change: func(o *spinner.Options) { o.Env = nil }},
		{name: "nil Env on windows", change: func(o *spinner.Options) {
			o.GOOS = "windows"
			o.Env = nil
		}, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := spinner.Options{
				Out:           &bytes.Buffer{},
				Width:         func() int { return 20 },
				Env:           map[string]string{"TERM": "xterm"},
				GOOS:          "linux",
				OutIsTTY:      true,
				LogsForHumans: true,
			}
			if tc.change != nil {
				tc.change(&opts)
			}

			assert.Equal(t, tc.want, spinner.New(opts).Animated())
		})
	}
}
