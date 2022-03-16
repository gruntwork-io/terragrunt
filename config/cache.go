package config

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/gruntwork-io/terragrunt/options"
)

// StringCache - structure to store cached values
type StringCache struct {
	Cache map[string]string
	Mutex *sync.Mutex
}

// NewStringCache - create new string cache
func NewStringCache() *StringCache {
	return &StringCache{
		Cache: map[string]string{},
		Mutex: &sync.Mutex{},
	}
}

// Get - get cached value, md5 hash is used as key to have fixed length keys and avoid duplicates
func (cache *StringCache) Get(key string) (string, bool) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	value, found := cache.Cache[cacheKey]
	return value, found
}

// Put - put value in cache, md5 hash is used as key to have fixed length keys and avoid duplicates
func (cache *StringCache) Put(key string, value string) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	cache.Cache[cacheKey] = value
}

// IAMRoleOptionsCache - cache for IAMRole options
type IAMRoleOptionsCache struct {
	Cache map[string]options.IAMRoleOptions
	Mutex *sync.Mutex
}

// NewIAMRoleOptionsCache - create new cache for IAM roles
func NewIAMRoleOptionsCache() *IAMRoleOptionsCache {
	return &IAMRoleOptionsCache{
		Cache: map[string]options.IAMRoleOptions{},
		Mutex: &sync.Mutex{},
	}
}

// Get - get cached value, md5 hash is used as key to have fixed length keys and avoid duplicates
func (cache *IAMRoleOptionsCache) Get(key string) (options.IAMRoleOptions, bool) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	value, found := cache.Cache[cacheKey]
	return value, found
}

// Put - put value in cache, md5 hash is used as key to have fixed length keys and avoid duplicates
func (cache *IAMRoleOptionsCache) Put(key string, value options.IAMRoleOptions) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	cache.Cache[cacheKey] = value
}
