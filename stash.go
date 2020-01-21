package stash

import "time"

import "sync/atomic"

import "sync"

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
	size      int64
	mu        sync.RWMutex
	hot       map[string]int64
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
	s.memCache.Delete(k)
	return nil
}

func (s *Stash) Get(k string) (interface{}, error) {
	// first check if it's in memory
	v, ok := s.memCache.Get(k)
	if ok {
		return v, nil
	} else {
		// not in memory so we check the disk
		v, size, err := s.diskCache.GetWithSize(k)
		if err != nil {
			_ = size
			_ = v
			return nil, err
		}
		// check if we can add it to the mem cache
		if s.size+size < s.maxSize {
			err := s.memCache.Add(k, v, 48*time.Hour)
			if err != nil {
				return nil, err
			}
			atomic.AddInt64(&s.size, size)
			s.mu.Lock()
			s.hot[k]++
			s.mu.Unlock()
		}
	}
	return nil, nil
}
