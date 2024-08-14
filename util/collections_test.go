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

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.MatchesAny(tt.list, tt.element)
			assert.Equal(t, tt.expected, actual, "For list %v and element %s", tt.list, tt.element)
		})
	}
}

func TestListContainsElement(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListContainsElement(tt.list, tt.element)
			assert.Equal(t, tt.expected, actual, "For list %v and element %s", tt.list, tt.element)
		})
	}
}

func TestListEquals(t *testing.T) {
	t.Parallel()

	tc := []struct {
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
	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListEquals(tt.a, tt.b)
			assert.Equal(t, tt.expected, actual, "For list %v and list %v", tt.a, tt.b)
		})
	}
}

func TestListContainsSublist(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListContainsSublist(tt.list, tt.sublist)
			assert.Equal(t, tt.expected, actual, "For list %v and sublist %v", tt.list, tt.sublist)
		})
	}
}

func TestListHasPrefix(t *testing.T) {
	t.Parallel()

	tc := []struct {
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
	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ListHasPrefix(tt.list, tt.prefix)
			assert.Equal(t, tt.expected, actual, "For list %v and prefix %v", tt.list, tt.prefix)
		})
	}
}

func TestRemoveElementFromList(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.RemoveElementFromList(tt.list, tt.element)
			assert.Equal(t, tt.expected, actual, "For list %v and element %s", tt.list, tt.element)
		})
	}
}

func TestRemoveDuplicatesFromList(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			f := util.RemoveDuplicatesFromList[[]string]
			if tt.reverse {
				f = util.RemoveDuplicatesFromListKeepLast[[]string]
			}
			assert.Equal(t, tt.expected, f(tt.list), "For list %v", tt.list)
			t.Logf("%v passed", tt.list)
		})
	}
}

func TestCommaSeparatedStrings(t *testing.T) {
	t.Parallel()

	tc := []struct {
		list     []string
		expected string
	}{
		{[]string{}, ``},
		{[]string{"foo"}, `"foo"`},
		{[]string{"foo", "bar"}, `"foo", "bar"`},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, util.CommaSeparatedStrings(tt.list), "For list %v", tt.list)
			t.Logf("%v passed", tt.list)
		})
	}
}

func TestStringListInsert(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, util.StringListInsert(tt.list, tt.element, tt.index), "For list %v", tt.list)
			t.Logf("%v passed", tt.list)
		})
	}
}
