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
	mem     *MemoryCache
	disk    *DiskCache
	maxSize int64
}

func NewStash() *Stash {
	return nil
}

func (s *Stash) Set(k string, x interface{}, d time.Duration) {

}

func (s *Stash) Get(key string) (interface{}, bool) {
	return nil, false
}
