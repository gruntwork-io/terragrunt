package cache

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/gruntwork-io/terragrunt/options"
)

type CacheKey interface {
	string
}

type CacheValue interface {
	string | options.IAMRoleOptions
}

type GenericCache[cacheValue CacheValue] struct {
	Cache map[string]cacheValue
	Mutex *sync.Mutex
}

// NewGenericCache - create new generic cache
func NewGenericCache[cacheValue CacheValue]() *GenericCache[cacheValue] {
	return &GenericCache[cacheValue]{
		Cache: map[string]cacheValue{},
		Mutex: &sync.Mutex{},
	}
}

// Get - get cached value, sha256 hash is used as key to have fixed length keys and avoid duplicates
func (cache *GenericCache[CacheValue]) Get(key string) (CacheValue, bool) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	value, found := cache.Cache[cacheKey]
	return value, found
}

// Put - put value in cache, sha256 hash is used as key to have fixed length keys and avoid duplicates
func (cache *GenericCache[cacheValue]) Put(key string, value cacheValue) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	cache.Cache[cacheKey] = value
}
