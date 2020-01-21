package stash

import "time"

const (
	// For use with functions that take an expiration time.
	NoExpiration time.Duration = -1
	// For use with functions that take an expiration time. Equivalent to
	// passing in the same expiration duration as was given to New() or
	// NewFrom() when the cache was created (e.g. 5 minutes.)
	DefaultExpiration time.Duration = 0
)

type Stash struct {
	memCache  *MemoryCache
	diskCache *DiskCache
	maxSize   int64
}

func NewStash(root string, maxSize int64, defaultExpiration, cleanupInterval time.Duration) *Stash {
	mc := NewMemoryCache(defaultExpiration, cleanupInterval)
	dc := NewDiskCache(root, defaultExpiration, cleanupInterval)
	s := &Stash{
		memCache:  mc,
		diskCache: dc,
		maxSize:   maxSize,
	}
	return s
}

func (s *Stash) Set(k string, v interface{}, d time.Duration) error {
	err := s.diskCache.Set(k, v, d)
	if err != nil {
		return err
	}
	return nil
}

func (s *Stash) Get(key string) (interface{}, bool) {
	return s.memCache.Get(key)
}

//unsafe.Sizeof(hmap)
