package stash

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/ristretto"
)

const (
	// For use with functions that take an expiration time.
	NoExpiration time.Duration = -1
	// For use with functions that take an expiration time. Equivalent to
	// passing in the same expiration duration as was given to New() or
	// NewFrom() when the cache was created (e.g. 5 minutes.)
	DefaultExpiration time.Duration = 0
)

type Stash struct {
	memoryCache       *ristretto.Cache
	diskCache         *badger.DB
	defaultExpiration time.Duration
}

func New(root string, maxSize int64, defaultExpiration time.Duration, metrics bool) (*Stash, error) {
	if maxSize < 1 || maxSize == 0 {
		return nil, fmt.Errorf("please pass a reasonable size for the in memory cache, i.e. '1 << 30' is 1GB")
	}
	memoryCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     maxSize, // maximum cost of cache, 1 << 30 is 1GB.
		BufferItems: 64,      // number of keys per Get buffer.
		Metrics:     metrics,
	})
	if err != nil {
		return nil, err
	}

	diskCache, err := badger.Open(badger.DefaultOptions(root))
	if err != nil {
		return nil, err
	}
	s := &Stash{
		memoryCache:       memoryCache,
		diskCache:         diskCache,
		defaultExpiration: defaultExpiration,
	}
	return s, nil
}

func (s *Stash) Set(k string, v interface{}, d time.Duration) error {
	err := s.diskCache.Update(func(txn *badger.Txn) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		e := badger.NewEntry([]byte(k), b).WithTTL(d)
		err = txn.SetEntry(e)
		return err
	})
	if err != nil {
		return err
	}

	s.memoryCache.Del([]byte(k))
	return nil
}

func (s *Stash) Get(k string, v interface{}) error {
	// first check if it's in memory
	i, ok := s.memoryCache.Get([]byte(k))
	if ok {
		b, ok := i.([]byte)
		if ok {
			err := json.Unmarshal(b, &v)
			if err != nil {
				return err
			}
			return nil
		}
	}

	// it's not in memory so we check the disk
	err := s.diskCache.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(k))
		if err != nil {
			return err
		}

		var valCopy []byte
		err = item.Value(func(val []byte) error {
			valCopy = append([]byte{}, val...)
			return nil
		})
		if err != nil {
			return err
		}

		err = json.Unmarshal(valCopy, &v)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ok = s.memoryCache.SetWithTTL([]byte(k), b, 0, s.defaultExpiration)
	if !ok {
		return fmt.Errorf("error setting %s=>%s", k, b)
	}

	return nil
}

func (s *Stash) Del(k string) error {
	s.memoryCache.Del([]byte(k))
	err := s.diskCache.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(k))
	})
	return err
}

func (s *Stash) GetMemoryCacheStats() string {
	return s.memoryCache.Metrics.String()
}
