package spinner_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/stretchr/testify/assert"
)

// TestNewAnimated checks which terminals and environments get the progress line.
func TestNewAnimated(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		change func(v *venv.Venv) *venv.Venv
		opts   *spinner.Options
		name   string
		want   bool
	}{
		{name: "baseline", want: true},
		{name: "stderr is not a terminal", change: func(v *venv.Venv) *venv.Venv {
			v.Terminal.StderrIsTTY = func() bool { return false }

			return v
		}},
		{name: "console without escape sequences", change: func(v *venv.Venv) *venv.Venv {
			v.Terminal.StderrHandlesEscapes = func() bool { return false }

			return v
		}},
		{name: "logs not for humans", opts: &spinner.Options{}},
		{name: "width 0", change: withWidth(0)},
		{name: "width 19", change: withWidth(19)},
		{name: "width 20", change: withWidth(20), want: true},
		{name: "empty TERM on linux", change: withEnv("TERM", "")},
		{name: "empty TERM on windows", change: func(v *venv.Venv) *venv.Venv {
			return withEnv("TERM", "")(v).WithGOOS("windows")
		}, want: true},
		{name: "dumb TERM on linux", change: withEnv("TERM", "dumb")},
		{name: "dumb TERM on windows", change: func(v *venv.Venv) *venv.Venv {
			return withEnv("TERM", "dumb")(v).WithGOOS("windows")
		}},
		{name: "uppercase DUMB", change: withEnv("TERM", "DUMB")},
		{name: "CI true", change: withEnv("CI", "true")},
		{name: "CI 1", change: withEnv("CI", "1")},
		{name: "CI false", change: withEnv("CI", "false"), want: true},
		{name: "CI 0", change: withEnv("CI", "0"), want: true},
		{name: "CI empty", change: withEnv("CI", ""), want: true},
		{name: "CI spaced false", change: withEnv("CI", " false "), want: true},
		{name: "CI no", change: withEnv("CI", "no"), want: true},
		{name: "CI OFF", change: withEnv("CI", "OFF"), want: true},
		{name: "TF_BUILD", change: withEnv("TF_BUILD", "True")},
		{name: "TEAMCITY_VERSION", change: withEnv("TEAMCITY_VERSION", "1")},
		{name: "JENKINS_URL", change: withEnv("JENKINS_URL", "url")},
		{name: "TF_IN_AUTOMATION", change: withEnv("TF_IN_AUTOMATION", "1")},
		{name: "darwin", change: func(v *venv.Venv) *venv.Venv {
			return withEnv("TERM", "xterm-256color")(v).WithGOOS("darwin")
		}, want: true},
		{name: "empty Env on linux", change: func(v *venv.Venv) *venv.Venv {
			return v.WithEnv(map[string]string{})
		}},
		{name: "empty Env on windows", change: func(v *venv.Venv) *venv.Venv {
			return v.WithEnv(map[string]string{}).WithGOOS("windows")
		}, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().
				WithErrWriter(new(bytes.Buffer)).
				WithEnv(map[string]string{"TERM": "xterm"}).
				WithGOOS("linux")

			v.Terminal.StderrIsTTY = func() bool { return true }
			v.Terminal.StderrHandlesEscapes = func() bool { return true }
			v.Terminal.ErrWidth = func() int { return 20 }

			if tc.change != nil {
				v = tc.change(v)
			}

			opts := spinner.Options{LogsForHumans: true}
			if tc.opts != nil {
				opts = *tc.opts
			}

			assert.Equal(t, tc.want, spinner.New(v, opts).Animated())
		})
	}
}

func withWidth(width int) func(v *venv.Venv) *venv.Venv {
	return func(v *venv.Venv) *venv.Venv {
		v.Terminal.ErrWidth = func() int { return width }

		return v
	}
}

func withEnv(name, value string) func(v *venv.Venv) *venv.Venv {
	return func(v *venv.Venv) *venv.Venv {
		v.Env[name] = value

		return v
	}
}
