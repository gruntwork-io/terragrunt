package log

import (
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/sirupsen/logrus"
)

// These are the different logging levels.
const (
	//
	StderrLevel Level = iota
	//
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

// A constant exposing all logging levels
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
	if name, ok := levelNames[level]; ok {
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

var logrusLevels = map[Level]logrus.Level{
	StderrLevel: logrus.Level(1),
	StdoutLevel: logrus.Level(2),
	ErrorLevel:  logrus.Level(3),
	WarnLevel:   logrus.Level(4),
	InfoLevel:   logrus.Level(5),
	DebugLevel:  logrus.Level(6),
	TraceLevel:  logrus.Level(7),
}

type Levels []Level

func (levels Levels) ToLogrusLevels() []logrus.Level {
	logrusLevels := make([]logrus.Level, len(levels))

	for i, level := range levels {
		logrusLevels[i] = level.ToLogrusLevel()
	}

	return logrusLevels
}

func (levels Levels) Names() []string {
	strs := make([]string, len(levels))

	for i, level := range levels {
		strs[i] = level.String()
	}

	return strs
}

func (levels Levels) String() string {
	return strings.Join(levels.Names(), ", ")
}

func (level Level) ToLogrusLevel() logrus.Level {
	if logrusLevel, ok := logrusLevels[level]; ok {
		return logrusLevel
	}

	return logrus.Level(0)
}

func FromLogrusLevel(lvl logrus.Level) Level {
	for level, logrusLevel := range logrusLevels {
		if logrusLevel == lvl {
			return level
		}
	}

	return Level(0)
}
