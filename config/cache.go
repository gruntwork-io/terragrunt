package config

import (
	"crypto/md5"
	"fmt"
	"sync"
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

// Get - get cached value
func (cache *StringCache) Get(key string) (string, bool) {
	md5Sum := md5.Sum([]byte(key))
	cacheKey := fmt.Sprintf("%x", md5Sum)
	value, found := cache.Cache[cacheKey]
	return value, found
}

// Put - put value in cache
func (cache *StringCache) Put(key string, value string) {
	cache.Mutex.Lock()
	md5Sum := md5.Sum([]byte(key))
	cacheKey := fmt.Sprintf("%x", md5Sum)
	cache.Cache[cacheKey] = value
	cache.Mutex.Unlock()
}
