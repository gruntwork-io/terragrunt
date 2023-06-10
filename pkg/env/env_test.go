package env

import (
	"fmt"
	"os"
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
		// true
		{"", false, false},
		{"false", false, false},
		{"  false  ", false, false},
		{"False", false, false},
		{"FALSE", false, false},
		{"0", false, false},
		// false
		{"true", false, true},
		{"  true  ", false, true},
		{"True", false, true},
		{"TRUE", false, true},
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

			runTestCase := func() {
				actual := GetBoolEnv(envVarName, testCase.fallback)
				assert.Equal(t, testCase.expected, actual)
			}

			// first try to test fallback with missing env variable
			if testCase.envVarValue == "" {
				runTestCase()
			}

			os.Setenv(envVarName, testCase.envVarValue)
			defer os.Unsetenv(envVarName)
			runTestCase()

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

			runTestCase := func() {
				actual := GetIntEnv(envVarName, testCase.fallback)
				assert.Equal(t, testCase.expected, actual)
			}

			// first try to test fallback with missing env variable
			if testCase.envVarValue == "" {
				runTestCase()
			}

			os.Setenv(envVarName, testCase.envVarValue)
			defer os.Unsetenv(envVarName)
			runTestCase()

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

			runTestCase := func() {
				actual := GetStringEnv(envVarName, testCase.fallback)
				assert.Equal(t, testCase.expected, actual)
			}

			// first try to test fallback with missing env variable
			if testCase.envVarValue == "" {
				runTestCase()
			}

			os.Setenv(envVarName, testCase.envVarValue)
			defer os.Unsetenv(envVarName)
			runTestCase()

		})
	}
}
