package config

import (
	"fmt"
	"sync"
)

// TerragruntConfigCache - structure to store cached values
type TerragruntConfigCache struct {
	Cache map[string]TerragruntConfig
	Mutex *sync.Mutex
}

// NewTerragruntConfigCache - create new TerragruntConfig cache
func NewTerragruntConfigCache() *TerragruntConfigCache {
	return &TerragruntConfigCache{
		Cache: map[string]TerragruntConfig{},
		Mutex: &sync.Mutex{},
	}
}

// Get - get cached value
// Design decision: Drop the sha256 because map is already a hashtable
// See https://go.dev/src/runtime/map.go
func (cache *TerragruntConfigCache) Get(key string) (TerragruntConfig, bool) {
	keyAsByte := []byte(key)
	cacheKey := fmt.Sprintf("%x", keyAsByte)

	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	value, found := cache.Cache[cacheKey]
	return value, found
}

// Put - put value in cache
func (cache *TerragruntConfigCache) Put(key string, value TerragruntConfig) {
	keyAsByte := []byte(key)
	cacheKey := fmt.Sprintf("%x", keyAsByte)

	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	cache.Cache[cacheKey] = value
}
