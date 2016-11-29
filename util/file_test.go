package util

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestGetPathRelativeTo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path	 string
		basePath string
		expected string
	}{
		{"", "", "."},
		{"/root", "/root", "."},
		{"/root", "/root/child", ".."},
		{"/root", "/root/child/sub-child/sub-sub-child", "../../.."},
		{"/root/other-child", "/root/child", "../other-child"},
		{"/root/other-child/sub-child", "/root/child/sub-child", "../../other-child/sub-child"},
		{"/root", "/other-root", "../root"},
		{"/root", "/other-root/sub-child/sub-sub-child", "../../../root"},
	}

	for _, testCase := range testCases {
		actual, err := GetPathRelativeTo(testCase.path, testCase.basePath)
		assert.Nil(t, err, "Unexpected error for path %s and basePath %s: %v", testCase.path, testCase.basePath, err)
		assert.Equal(t, testCase.expected, actual, "For path %s and basePath %s", testCase.path, testCase.basePath)
	}
}
