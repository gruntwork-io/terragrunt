package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchesAny(t *testing.T) {
	t.Parallel()

	realWorldErrorMessages := []string{
		"Failed to load state: RequestError: send request failed\ncaused by: Get https://<BUCKET_NAME>.us-west-2.amazonaws.com/?prefix=env%3A%2F: dial tcp 54.231.176.160:443: i/o timeout",
		"aws_cloudwatch_metric_alarm.asg_high_memory_utilization: Creating metric alarm failed: ValidationError: A separate request to update this alarm is in progress. status code: 400, request id: 94309fbd-7e09-11e8-a5f8-1de9e697c6bf",
		"Error configuring the backend \"s3\": RequestError: send request failed\ncaused by: Post https://sts.amazonaws.com/: net/http: TLS handshake timeout",
	}

	testCases := []struct {
		list     []string
		element  string
		expected bool
	}{
		{nil, "", false},
		{[]string{}, "", false},
		{[]string{}, "foo", false},
		{[]string{"foo"}, "kafoot", true},
		{[]string{"bar", "foo", ".*Failed to load backend.*TLS handshake timeout.*"}, "Failed to load backend: Error...:...  TLS handshake timeout", true},
		{[]string{"bar", "foo", ".*Failed to load backend.*TLS handshake timeout.*"}, "Failed to load backend: Error...:...  TLxS handshake timeout", false},
		{[]string{"(?s).*Failed to load state.*dial tcp.*timeout.*"}, realWorldErrorMessages[0], true},
		{[]string{"(?s).*Creating metric alarm failed.*request to update this alarm is in progress.*"}, realWorldErrorMessages[1], true},
		{[]string{"(?s).*Error configuring the backend.*TLS handshake timeout.*"}, realWorldErrorMessages[2], true},
	}

	for _, testCase := range testCases {
		actual := MatchesAny(testCase.list, testCase.element)
		assert.Equal(t, testCase.expected, actual, "For list %v and element %s", testCase.list, testCase.element)
	}
}

func TestListContainsElement(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		element  string
		expected bool
	}{
		{[]string{}, "", false},
		{[]string{}, "foo", false},
		{[]string{"foo"}, "foo", true},
		{[]string{"bar", "foo", "baz"}, "foo", true},
		{[]string{"bar", "foo", "baz"}, "nope", false},
		{[]string{"bar", "foo", "baz"}, "", false},
	}

	for _, testCase := range testCases {
		actual := ListContainsElement(testCase.list, testCase.element)
		assert.Equal(t, testCase.expected, actual, "For list %v and element %s", testCase.list, testCase.element)
	}
}

func TestListEquals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		a        []string
		b        []string
		expected bool
	}{
		{[]string{""}, []string{}, false},
		{[]string{"foo"}, []string{"bar"}, false},
		{[]string{"foo", "bar"}, []string{"bar"}, false},
		{[]string{"foo"}, []string{"foo", "bar"}, false},
		{[]string{"foo", "bar"}, []string{"bar", "foo"}, false},

		{[]string{}, []string{}, true},
		{[]string{""}, []string{""}, true},
		{[]string{"foo", "bar"}, []string{"foo", "bar"}, true},
	}
	for _, testCase := range testCases {
		actual := ListEquals(testCase.a, testCase.b)
		assert.Equal(t, testCase.expected, actual, "For list %v and list %v", testCase.a, testCase.b)
	}
}

func TestListContainsSublist(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		sublist  []string
		expected bool
	}{
		{[]string{}, []string{}, false},
		{[]string{}, []string{"foo"}, false},
		{[]string{"foo"}, []string{}, false},
		{[]string{"foo"}, []string{"bar"}, false},
		{[]string{"foo"}, []string{"foo", "bar"}, false},
		{[]string{"bar", "foo"}, []string{"foo", "bar"}, false},
		{[]string{"bar", "foo", "gee"}, []string{"foo", "bar"}, false},
		{[]string{"foo", "foo", "gee"}, []string{"foo", "bar"}, false},
		{[]string{"zim", "gee", "foo", "foo", "foo"}, []string{"foo", "foo", "bar", "bar"}, false},

		{[]string{""}, []string{""}, true},
		{[]string{"foo"}, []string{"foo"}, true},
		{[]string{"foo", "bar"}, []string{"foo"}, true},
		{[]string{"bar", "foo"}, []string{"foo"}, true},
		{[]string{"foo", "bar", "gee"}, []string{"foo", "bar"}, true},
		{[]string{"zim", "foo", "bar", "gee"}, []string{"foo", "bar"}, true},
		{[]string{"foo", "foo", "bar", "gee"}, []string{"foo", "bar"}, true},
		{[]string{"zim", "gee", "foo", "bar"}, []string{"foo", "bar"}, true},
		{[]string{"foo", "foo", "foo", "bar"}, []string{"foo", "foo"}, true},
		{[]string{"bar", "foo", "foo", "foo"}, []string{"foo", "foo"}, true},
		{[]string{"zim", "gee", "foo", "bar"}, []string{"gee", "foo", "bar"}, true},
	}

	for _, testCase := range testCases {
		actual := ListContainsSublist(testCase.list, testCase.sublist)
		assert.Equal(t, testCase.expected, actual, "For list %v and sublist %v", testCase.list, testCase.sublist)
	}
}

func TestListHasPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		prefix   []string
		expected bool
	}{
		{[]string{}, []string{}, false},
		{[]string{""}, []string{}, false},
		{[]string{"foo"}, []string{"bar"}, false},
		{[]string{"foo", "bar"}, []string{"bar"}, false},
		{[]string{"foo"}, []string{"foo", "bar"}, false},
		{[]string{"foo", "bar", "foo"}, []string{"bar", "foo"}, false},

		{[]string{""}, []string{""}, true},
		{[]string{"", "foo"}, []string{""}, true},
		{[]string{"foo", "bar"}, []string{"foo"}, true},
		{[]string{"foo", "bar"}, []string{"foo", "bar"}, true},
		{[]string{"foo", "bar", "biz"}, []string{"foo", "bar"}, true},
	}
	for _, testCase := range testCases {
		actual := ListHasPrefix(testCase.list, testCase.prefix)
		assert.Equal(t, testCase.expected, actual, "For list %v and prefix %v", testCase.list, testCase.prefix)
	}
}

func TestRemoveElementFromList(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		element  string
		expected []string
	}{
		{[]string{}, "", []string{}},
		{[]string{}, "foo", []string{}},
		{[]string{"foo"}, "foo", []string{}},
		{[]string{"bar"}, "foo", []string{"bar"}},
		{[]string{"bar", "foo", "baz"}, "foo", []string{"bar", "baz"}},
		{[]string{"bar", "foo", "baz"}, "nope", []string{"bar", "foo", "baz"}},
		{[]string{"bar", "foo", "baz"}, "", []string{"bar", "foo", "baz"}},
	}

	for _, testCase := range testCases {
		actual := RemoveElementFromList(testCase.list, testCase.element)
		assert.Equal(t, testCase.expected, actual, "For list %v and element %s", testCase.list, testCase.element)
	}
}

func TestRemoveDuplicatesFromList(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		expected []string
		reverse  bool
	}{
		{[]string{}, []string{}, false},
		{[]string{"foo"}, []string{"foo"}, false},
		{[]string{"foo", "bar"}, []string{"foo", "bar"}, false},
		{[]string{"foo", "bar", "foobar", "bar", "foo"}, []string{"foo", "bar", "foobar"}, false},
		{[]string{"foo", "bar", "foobar", "foo", "bar"}, []string{"foo", "bar", "foobar"}, false},
		{[]string{"foo", "bar", "foobar", "bar", "foo"}, []string{"foobar", "bar", "foo"}, true},
		{[]string{"foo", "bar", "foobar", "foo", "bar"}, []string{"foobar", "foo", "bar"}, true},
	}

	for _, testCase := range testCases {
		f := RemoveDuplicatesFromList
		if testCase.reverse {
			f = RemoveDuplicatesFromListKeepLast
		}
		assert.Equal(t, f(testCase.list), testCase.expected, "For list %v", testCase.list)
		t.Logf("%v passed", testCase.list)
	}
}

func TestCommaSeparatedStrings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		expected string
	}{
		{[]string{}, ``},
		{[]string{"foo"}, `"foo"`},
		{[]string{"foo", "bar"}, `"foo", "bar"`},
	}

	for _, testCase := range testCases {
		assert.Equal(t, CommaSeparatedStrings(testCase.list), testCase.expected, "For list %v", testCase.list)
		t.Logf("%v passed", testCase.list)
	}
}

func TestStringListInsert(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		element  string
		index    int
		expected []string
	}{
		{[]string{}, "foo", 0, []string{"foo"}},
		{[]string{"a", "c", "d"}, "b", 1, []string{"a", "b", "c", "d"}},
		{[]string{"b", "c", "d"}, "a", 0, []string{"a", "b", "c", "d"}},
		{[]string{"a", "b", "d"}, "c", 2, []string{"a", "b", "c", "d"}},
	}

	for _, testCase := range testCases {
		assert.Equal(t, testCase.expected, StringListInsert(testCase.list, testCase.element, testCase.index), "For list %v", testCase.list)
		t.Logf("%v passed", testCase.list)
	}
}

func TestKeyValuePairStringListToMap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    []string
		splitter func(s, sep string) []string
		output   map[string]string
	}{
		{
			"base",
			[]string{"foo=bar", "baz=carol"},
			SplitUrls,
			map[string]string{
				"foo": "bar",
				"baz": "carol",
			},
		},
		{
			"special_chars",
			[]string{"ssh://git@github.com=/path/to/local"},
			SplitUrls,
			map[string]string{"ssh://git@github.com": "/path/to/local"},
		},
		{
			"with_tags",
			[]string{"ssh://git@github.com=ssh://git@github.com/test.git?ref=feature"},
			SplitUrls,
			map[string]string{"ssh://git@github.com": "ssh://git@github.com/test.git?ref=feature"},
		},
		{
			"empty",
			[]string{},
			SplitUrls,
			map[string]string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualOutput, err := KeyValuePairStringListToMap(testCase.input, testCase.splitter)
			assert.NoError(t, err)
			assert.Equal(
				t,
				testCase.output,
				actualOutput,
			)
		})
	}
}
