package log

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/sirupsen/logrus"
)

// These are the different logging levels.
const (
	// StderrLevel level. Used to log error messages that we get from OpenTofu/Terraform stderr.
	StderrLevel Level = iota
	// StdoutLevel level. Used to log messages that we get from OpenTofu/Terraform stdout.
	StdoutLevel
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	ErrorLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// InfoLevel level. General operational entries about what's going on inside the application.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
	// TraceLevel level. Designates finer-grained informational events than the Debug.
	TraceLevel
)

// Since the first two logrus levels are Panic and Fatal, which cause an exit or panic when called, we need to shift all of our levels by two bytes.
const shiftLogrusLevel = 2

var logrusLevels = map[Level]logrus.Level{
	StderrLevel: logrus.Level(StderrLevel + shiftLogrusLevel),
	StdoutLevel: logrus.Level(StdoutLevel + shiftLogrusLevel),
	ErrorLevel:  logrus.Level(ErrorLevel + shiftLogrusLevel),
	WarnLevel:   logrus.Level(WarnLevel + shiftLogrusLevel),
	InfoLevel:   logrus.Level(InfoLevel + shiftLogrusLevel),
	DebugLevel:  logrus.Level(DebugLevel + shiftLogrusLevel),
	TraceLevel:  logrus.Level(TraceLevel + shiftLogrusLevel),
}

// AllLevels exposes all logging levels
var AllLevels = Levels{
	StderrLevel,
	StdoutLevel,
	ErrorLevel,
	WarnLevel,
	InfoLevel,
	DebugLevel,
	TraceLevel,
}

var levelNames = map[Level]string{
	StderrLevel: "stderr",
	StdoutLevel: "stdout",
	ErrorLevel:  "error",
	WarnLevel:   "warn",
	InfoLevel:   "info",
	DebugLevel:  "debug",
	TraceLevel:  "trace",
}

var levelShortNames = map[Level]string{
	StderrLevel: "std",
	StdoutLevel: "std",
	ErrorLevel:  "err",
	WarnLevel:   "wrn",
	InfoLevel:   "inf",
	DebugLevel:  "deb",
	TraceLevel:  "trc",
}

var levelTinyNames = map[Level]string{
	StderrLevel: "s",
	StdoutLevel: "s",
	ErrorLevel:  "e",
	WarnLevel:   "w",
	InfoLevel:   "i",
	DebugLevel:  "d",
	TraceLevel:  "t",
}

// Level type
type Level uint32

// ParseLevel takes a string and returns the Level constant.
func ParseLevel(str string) (Level, error) {
	for level, name := range levelNames {
		if strings.EqualFold(name, str) {
			return level, nil
		}
	}

	return Level(0), errors.Errorf("invalid level %q, supported levels: %s", str, AllLevels)
}

// String implements fmt.Stringer.
func (level Level) String() string {
	return level.FullName()
}

// FullName returns the full level name.
func (level Level) FullName() string {
	if name, ok := levelNames[level]; ok {
		return name
	}

	return ""
}

// TinyName returns the level name in one character.
func (level Level) TinyName() string {
	if name, ok := levelTinyNames[level]; ok {
		return name
	}

	return ""
}

// ShortName returns the level name in third characters.
func (level Level) ShortName() string {
	if name, ok := levelShortNames[level]; ok {
		return name
	}

	return ""
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (level *Level) UnmarshalText(text []byte) error {
	lvl, err := ParseLevel(string(text))
	if err != nil {
		return errors.Errorf("invalid: %q", string(text))
	}

	*level = lvl

	return nil
}

// MarshalText implements encoding.MarshalText.
func (level Level) MarshalText() ([]byte, error) {
	if name := level.String(); name != "" {
		return []byte(name), nil
	}

	return nil, errors.Errorf("invalid: %q", level)
}

// Levels is a slice of `Level` type.
type Levels []Level

// Contains returns true if the `Levels` list contains the given search `Level`.
func (levels Levels) Contains(search Level) bool {
	for _, level := range levels {
		if level == search {
			return true
		}
	}

	return false
}

// ToLogrusLevels converts our `Levels` to `logrus.Levels`.
func (levels Levels) ToLogrusLevels() []logrus.Level {
	logrusLevels := make([]logrus.Level, len(levels))

	for i, level := range levels {
		logrusLevels[i] = level.ToLogrusLevel()
	}

	return logrusLevels
}

// Names returns a list of full level names.
func (levels Levels) Names() []string {
	strs := make([]string, len(levels))

	for i, level := range levels {
		strs[i] = level.String()
	}

	return strs
}

// String implements the `fmt.Stringer` interface.
func (levels Levels) String() string {
	return strings.Join(levels.Names(), ", ")
}

// ToLogrusLevel converts our `Level` to `logrus.Level`.
func (level Level) ToLogrusLevel() logrus.Level {
	if logrusLevel, ok := logrusLevels[level]; ok {
		return logrusLevel
	}

	return logrus.Level(0)
}

// FromLogrusLevel converts `logrus.Level` to our `Level`.
func FromLogrusLevel(lvl logrus.Level) Level {
	for level, logrusLevel := range logrusLevels {
		if logrusLevel == lvl {
			return level
		}
	}

	return Level(0)
}
