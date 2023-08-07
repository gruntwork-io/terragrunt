package env

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBoolEnv(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		envVarValue string
		fallback    bool
		expected    bool
	}{
		// false
		{"", false, false},
		{"false", false, false},
		{"  false  ", false, false},
		{"False", false, false},
		{"FALSE", false, false},
		{"0", false, false},
		// true
		{"true", false, true},
		{"  true  ", false, true},
		{"True", false, true},
		{"TRUE", false, true},
		{"", true, true},
		{"", true, true},
		{"1", true, true},
		{"foo", false, false},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("TestGetBoolEnv-testCase-%d", i)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			actual := GetBoolEnv(testCase.envVarValue, testCase.fallback)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestGetNegativeBoolEnv(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		envVarValue string
		fallback    bool
		expected    bool
	}{
		// true
		{"", true, true},
		{"false", false, true},
		{"  false  ", false, true},
		{"False", false, true},
		{"FALSE", false, true},
		{"0", false, true},
		// false
		{"", false, false},
		{"true", false, false},
		{"  true  ", false, false},
		{"True", false, false},
		{"TRUE", false, false},

		{"1", true, false},
		{"foo", false, false},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("TestGetNegativeBoolEnv-testCase-%d", i)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			actual := GetNegativeBoolEnv(testCase.envVarValue, testCase.fallback)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestGetIntEnv(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		envVarValue string
		fallback    int
		expected    int
	}{
		{"10", 20, 10},
		{"0", 30, 0},
		{"", 5, 5},
		{"foo", 15, 15},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("TestGetIntEnv-testCase-%d", i)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			actual := GetIntEnv(testCase.envVarValue, testCase.fallback)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestGetStringEnv(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		envVarValue string
		fallback    string
		expected    string
	}{
		{"first", "second", "first"},
		{"", "second", "second"},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("test-%d-val-%s-expected-%s", i, testCase.envVarValue, testCase.expected)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			actual := GetStringEnv(testCase.envVarValue, testCase.fallback)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestParseEnvironmentVariables(t *testing.T) {
	testCases := []struct {
		environmentVariables []string
		expectedVariables    map[string]string
	}{
		{
			[]string{},
			map[string]string{},
		},
		{
			[]string{"foobar"},
			map[string]string{},
		},
		{
			[]string{"foo=bar"},
			map[string]string{"foo": "bar"},
		},
		{
			[]string{"foo=bar", "goo=gar"},
			map[string]string{"foo": "bar", "goo": "gar"},
		},
		{
			[]string{"foo=bar   "},
			map[string]string{"foo": "bar   "},
		},
		{
			[]string{"foo   =bar   "},
			map[string]string{"foo": "bar   "},
		},
		{
			[]string{"foo=composite=bar"},
			map[string]string{"foo": "composite=bar"},
		},
	}

	for _, testCase := range testCases {
		actualVariables := ParseEnvs(testCase.environmentVariables)
		assert.Equal(t, testCase.expectedVariables, actualVariables)
	}
}
