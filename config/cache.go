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

// Get - get cached value, md5 hash is used as key to have fixed length keys and avoid duplicates
func (cache *StringCache) Get(key string) (string, bool) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	md5Sum := md5.Sum([]byte(key))
	cacheKey := fmt.Sprintf("%x", md5Sum)
	value, found := cache.Cache[cacheKey]
	return value, found
}

// Put - put value in cache, md5 hash is used as key to have fixed length keys and avoid duplicates
func (cache *StringCache) Put(key string, value string) {
	cache.Mutex.Lock()
	defer cache.Mutex.Unlock()
	md5Sum := md5.Sum([]byte(key))
	cacheKey := fmt.Sprintf("%x", md5Sum)
	cache.Cache[cacheKey] = value
}
