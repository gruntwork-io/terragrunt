package util_test

import (
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
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
		element  string
		list     []string
		expected bool
	}{
		{list: nil, element: "", expected: false},
		{list: []string{}, element: "", expected: false},
		{list: []string{}, element: "foo", expected: false},
		{list: []string{"foo"}, element: "kafoot", expected: true},
		{list: []string{"bar", "foo", ".*Failed to load backend.*TLS handshake timeout.*"}, element: "Failed to load backend: Error...:...  TLS handshake timeout", expected: true},
		{list: []string{"bar", "foo", ".*Failed to load backend.*TLS handshake timeout.*"}, element: "Failed to load backend: Error...:...  TLxS handshake timeout", expected: false},
		{list: []string{"(?s).*Failed to load state.*dial tcp.*timeout.*"}, element: realWorldErrorMessages[0], expected: true},
		{list: []string{"(?s).*Creating metric alarm failed.*request to update this alarm is in progress.*"}, element: realWorldErrorMessages[1], expected: true},
		{list: []string{"(?s).*Error configuring the backend.*TLS handshake timeout.*"}, element: realWorldErrorMessages[2], expected: true},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.MatchesAny(tc.list, tc.element)
			assert.Equal(t, tc.expected, actual, "For list %v and element %s", tc.list, tc.element)
		})
	}
}

func TestListContainsElement(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		element  string
		list     []string
		expected bool
	}{
		{list: []string{}, element: "", expected: false},
		{list: []string{}, element: "foo", expected: false},
		{list: []string{"foo"}, element: "foo", expected: true},
		{list: []string{"bar", "foo", "baz"}, element: "foo", expected: true},
		{list: []string{"bar", "foo", "baz"}, element: "nope", expected: false},
		{list: []string{"bar", "foo", "baz"}, element: "", expected: false},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListContainsElement(tc.list, tc.element)
			assert.Equal(t, tc.expected, actual, "For list %v and element %s", tc.list, tc.element)
		})
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
	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListEquals(tc.a, tc.b)
			assert.Equal(t, tc.expected, actual, "For list %v and list %v", tc.a, tc.b)
		})
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

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListContainsSublist(tc.list, tc.sublist)
			assert.Equal(t, tc.expected, actual, "For list %v and sublist %v", tc.list, tc.sublist)
		})
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
	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListHasPrefix(tc.list, tc.prefix)
			assert.Equal(t, tc.expected, actual, "For list %v and prefix %v", tc.list, tc.prefix)
		})
	}
}

func TestRemoveElementFromList(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		list     []string
		element  string
		expected []string
	}{
		{[]string{}, "", nil},
		{[]string{}, "foo", nil},
		{[]string{"foo"}, "foo", nil},
		{[]string{"bar"}, "foo", []string{"bar"}},
		{[]string{"bar", "foo", "baz"}, "foo", []string{"bar", "baz"}},
		{[]string{"bar", "foo", "baz"}, "nope", []string{"bar", "foo", "baz"}},
		{[]string{"bar", "foo", "baz"}, "", []string{"bar", "foo", "baz"}},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.RemoveElementFromList(tc.list, tc.element)
			assert.Equal(t, tc.expected, actual, "For list %v and element %s", tc.list, tc.element)
		})
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

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			f := util.RemoveDuplicatesFromList[[]string]
			if tc.reverse {
				f = util.RemoveDuplicatesFromListKeepLast[[]string]
			}

			assert.Equal(t, tc.expected, f(tc.list), "For list %v", tc.list)
			t.Logf("%v passed", tc.list)
		})
	}
}

func TestCommaSeparatedStrings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expected string
		list     []string
	}{
		{list: []string{}, expected: ``},
		{list: []string{"foo"}, expected: `"foo"`},
		{list: []string{"foo", "bar"}, expected: `"foo", "bar"`},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, util.CommaSeparatedStrings(tc.list), "For list %v", tc.list)
			t.Logf("%v passed", tc.list)
		})
	}
}

func TestStringListInsert(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		element  string
		list     []string
		expected []string
		index    int
	}{
		{list: []string{}, element: "foo", index: 0, expected: []string{"foo"}},
		{list: []string{"a", "c", "d"}, element: "b", index: 1, expected: []string{"a", "b", "c", "d"}},
		{list: []string{"b", "c", "d"}, element: "a", index: 0, expected: []string{"a", "b", "c", "d"}},
		{list: []string{"a", "b", "d"}, element: "c", index: 2, expected: []string{"a", "b", "c", "d"}},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, util.StringListInsert(tc.list, tc.element, tc.index), "For list %v", tc.list)
			t.Logf("%v passed", tc.list)
		})
	}
}

func TestMapToSlice(t *testing.T) {
	t.Parallel()

	t.Run("Empty Map", func(t *testing.T) {
		t.Parallel()

		m := make(map[string]*int)

		result := util.MapToSlice(m)
		if len(result) != 0 {
			t.Errorf("Expected empty slice, got %v", result)
		}
	})

	t.Run("Single Element Map", func(t *testing.T) {
		t.Parallel()

		val := 42
		m := map[string]*int{"key1": &val}

		result := util.MapToSlice(m)
		if len(result) != 1 || result[0] != &val {
			t.Errorf("Expected slice with one element %v, got %v", &val, result)
		}
	})
}
