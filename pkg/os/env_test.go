package os

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBoolEnv(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		envVarValue   string
		fallback      bool
		expected      bool
		expectedError bool
	}{
		// true
		{"", false, false, false},
		{"false", false, false, false},
		{"  false  ", false, false, false},
		{"False", false, false, false},
		{"FALSE", false, false, false},
		{"0", false, false, false},
		// false
		{"true", false, true, false},
		{"  true  ", false, true, false},
		{"True", false, true, false},
		{"TRUE", false, true, false},
		{"", true, true, false},
		{"1", true, true, false},
		// error
		{"foo", false, false, true},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("TestGetBoolEnv-testCase-%d", i)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			runTestCase := func() {
				actual, err := GetBoolEnv(envVarName, testCase.fallback)
				if testCase.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
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
		envVarValue   string
		fallback      int
		expected      int
		expectedError bool
	}{
		{"1", 0, 1, false},
		{"0", 1, 0, false},
		{"", 1, 1, false},
		// error
		{"foo", 1, 0, true},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("TestGetIntEnv-testCase-%d", i)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			runTestCase := func() {
				actual, err := GetIntEnv(envVarName, testCase.fallback)
				if testCase.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
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

func TestGetStringSliceEnv(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		envVarValue string
		sep         string
		fallback    []string
		expected    []string
	}{
		{"first,second,third", ",", []string{"fourth", "fifth"}, []string{"first", "second", "third"}},
		{"    first :   second  :   third  ", ":", []string{"fourth", "fifth"}, []string{"first", "second", "third"}},
		{"", "", []string{"fourth", "fifth"}, []string{"fourth", "fifth"}},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("TestGetStringSliceEnv-testCase-%d", i)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			runTestCase := func() {
				actual := GetStringSliceEnv(envVarName, testCase.sep, strings.Split, testCase.fallback)
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

func TestGetStringMapEnv(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		envVarValue      string
		sliceSep, mapSep string
		fallback         map[string]string
		expected         map[string]string
		expectedError    bool
	}{
		{"color=white  ,  number  =  10  ", ",", "=", map[string]string{"color": "black", "number": "20"}, map[string]string{"color": "white", "number": "10"}, false},
		{"", ",", "=", map[string]string{"color": "black", "number": "20"}, map[string]string{"color": "black", "number": "20"}, false},
		{"color,number=10", ",", "=", map[string]string{"color": "black", "number": "20"}, nil, true},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		envVarName := fmt.Sprintf("TestGetStringMapEnv-testCase-%d", i)
		t.Run(envVarName, func(t *testing.T) {
			t.Parallel()

			runTestCase := func() {
				actual, err := GetStringMapEnv(envVarName, testCase.sliceSep, testCase.mapSep, strings.Split, testCase.fallback)
				if testCase.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
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
