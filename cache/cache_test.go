package cache

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestStringCacheCreation(t *testing.T) {
	t.Parallel()

	cache := NewGenericCache[string]()

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Equal(t, 0, len(cache.Cache))
}

func TestStringCacheOperation(t *testing.T) {
	t.Parallel()

	cache := NewGenericCache[string]()

	value, found := cache.Get("potato")

	assert.False(t, found)
	assert.Empty(t, value)

	cache.Put("potato", "carrot")
	value, found = cache.Get("potato")

	assert.True(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}

func TestIAMRoleOptionsCacheOperation(t *testing.T) {
	t.Parallel()

	cache := NewGenericCache[options.IAMRoleOptions]()

	value, found := cache.Get("option1")

	assert.False(t, found)
	assert.Equal(t, options.IAMRoleOptions{}, value)

	iamRoleOption := &options.IAMRoleOptions{RoleARN: "random"}

	cache.Put("option1", *iamRoleOption)
	value, found = cache.Get("option1")

	assert.True(t, found)
	assert.Equal(t, *iamRoleOption, value)
}
