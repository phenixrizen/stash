package stash

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/crypto/blake2b"
)

var cacheRoot string = "."

type key string

func (k key) Hash() string {
	h := blake2b.Sum256([]byte(k))
	return fmt.Sprintf("%x", h)
}

func (k key) Path() string {
	return cacheRoot + k.Hash()
}

type diskItem struct {
	Object     interface{}
	Expiration int64
	Key        key
	Size       int64
}

// Returns true if the item has expired.
func (item *diskItem) expired() bool {
	if item.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

// write the byes to disk
func (item *diskItem) write() error {
	if item.Object == nil {
		return fmt.Errorf("item has nil object")
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(item); err != nil {
		return err
	}

	data := buf.Bytes()
	err := ioutil.WriteFile(item.Key.Path(), data, 0644)
	if err != nil {
		return err
	}
	item.Object = nil
	item.Size = int64(len(data))

	return nil
}

// read the byes from disk
func (item *diskItem) read(i *diskItem) error {
	f, err := os.Open(item.Key.Path())
	if err != nil {
		return err
	}
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&i); err != nil {
		return err
	}
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	i.Size = stat.Size()

	return nil
}

// size returns the size in bytes of the item
func (item *diskItem) size() int64 {
	return item.Size
}

// delete the byes from disk
func (item *diskItem) delete() error {
	return os.Remove(item.Key.Path())
}

type DiskCache struct {
	*diskCache
}

type diskCache struct {
	root              string
	defaultExpiration time.Duration
	items             map[string]*diskItem
	mu                sync.RWMutex
	onEvicted         func(string, interface{})
	janitor           *diskJanitor
}

// Register, registers the types for Gob encoding or decoding
func (c *diskCache) Register(t interface{}) {
	gob.Register(t)
}

// Add an item to the cache, replacing any existing item. If the duration is 0
// (DefaultExpiration), the cache's default expiration time is used. If it is -1
// (NoExpiration), the item never expires.
func (c *diskCache) Set(k string, v interface{}, d time.Duration) error {
	var e int64
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	c.mu.Lock()
	item := &diskItem{
		Object:     v,
		Expiration: e,
		Key:        key(k),
	}
	err := item.write()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	item.Object = nil
	c.items[k] = item
	// TODO: Calls to mu.Unlock are currently not deferred because defer
	// adds ~200 ns (as of go1.)
	c.mu.Unlock()

	return nil
}

func (c *diskCache) set(k string, v interface{}, d time.Duration) error {
	var e int64
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	item := &diskItem{
		Object:     v,
		Expiration: e,
		Key:        key(k),
	}
	err := item.write()
	if err != nil {
		return err
	}
	item.Object = nil
	c.items[k] = item

	return nil
}

// Add an item to the cache, replacing any existing item, using the default
// expiration.
func (c *diskCache) SetDefault(k string, v interface{}) error {
	return c.Set(k, v, DefaultExpiration)
}

// Add an item to the cache only if an item doesn't already exist for the given
// key, or if the existing item has expired. Returns an error otherwise.
func (c *diskCache) Add(k string, v interface{}, d time.Duration) error {
	c.mu.Lock()
	_, found, err := c.get(k)
	if err != nil {
		return err
	}
	if found {
		c.mu.Unlock()
		return fmt.Errorf("Item %s already exists", k)
	}
	err = c.set(k, v, d)
	if err != nil {
		return err
	}
	c.mu.Unlock()
	return nil
}

// Set a new value for the cache key only if it already exists, and the existing
// item hasn't expired. Returns an error otherwise.
func (c *diskCache) Replace(k string, v interface{}, d time.Duration) error {
	c.mu.Lock()
	_, found, err := c.get(k)
	if err != nil {
		return err
	}
	if !found {
		c.mu.Unlock()
		return fmt.Errorf("Item %s doesn't exist", k)
	}
	c.set(k, v, d)
	c.mu.Unlock()
	return nil
}

// Get an item from the cache. Returns the item or nil, and a bool indicating
// whether the key was found.
func (c *diskCache) Get(k string) (interface{}, error) {
	c.mu.RLock()
	// "Inlining" of get and Expired
	item, found := c.items[k]
	if found {
		err := item.read(item)
		if err != nil {
			return nil, err
		}
	}
	if !found {
		c.mu.RUnlock()
		return nil, nil
	}
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return nil, nil
		}
	}
	c.mu.RUnlock()
	return item.Object, nil
}

// GetWithSize an item from the cache. Returns the item or nil, and a bool indicating
// whether the key was found.
func (c *diskCache) GetWithSize(k string) (interface{}, int64, error) {
	c.mu.RLock()
	// "Inlining" of get and Expired
	item, found := c.items[k]
	if found {
		err := item.read(item)
		if err != nil {
			return nil, -1, err
		}
	}
	if !found {
		c.mu.RUnlock()
		return nil, -1, nil
	}
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return nil, -1, nil
		}
	}
	c.mu.RUnlock()
	return item.Object, item.size(), nil
}

// GetWithExpiration returns an item and its expiration time from the cache.
// It returns the item or nil, the expiration time if one is set (if the item
// never expires a zero value for time.Time is returned), and a bool indicating
// whether the key was found.
func (c *diskCache) GetWithExpiration(k string) (interface{}, time.Time, bool) {
	c.mu.RLock()
	// "Inlining" of get and Expired
	item, found := c.items[k]
	if !found {
		c.mu.RUnlock()
		return nil, time.Time{}, false
	}

	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return nil, time.Time{}, false
		}

		// Return the item and the expiration time
		c.mu.RUnlock()
		return item.Object, time.Unix(0, item.Expiration), true
	}

	// If expiration <= 0 (i.e. no expiration time set) then return the item
	// and a zeroed time.Time
	c.mu.RUnlock()
	return item.Object, time.Time{}, true
}

func (c *diskCache) get(k string) (interface{}, bool, error) {
	item, found := c.items[k]
	if found {
		err := item.read(item)
		if err != nil {
			return nil, false, err
		}
	}
	if !found {
		return nil, false, nil
	}
	// "Inlining" of Expired
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			return nil, false, nil
		}
	}
	return item.Object, true, nil
}

// Delete an item from the cache. Does nothing if the key is not in the cache.
func (c *diskCache) Delete(k string) error {
	c.mu.Lock()
	v, evicted, err := c.delete(k)
	if err != nil {
		return err
	}
	c.mu.Unlock()
	if evicted {
		c.onEvicted(k, v)
	}

	return nil
}

func (c *diskCache) delete(k string) (interface{}, bool, error) {
	if c.onEvicted != nil {
		if v, found := c.items[k]; found {
			delete(c.items, k)
			err := v.delete()
			if err != nil {
				return v.Object, true, err
			}
			return v.Object, true, nil
		}
	}
	item := c.items[k]
	err := item.delete()
	if err != nil {
		return nil, false, err
	}
	delete(c.items, k)
	return nil, false, nil
}

type diskKeyAndValue struct {
	key   string
	value interface{}
}

// Delete all expired items from the cache.
func (c *diskCache) DeleteExpired() error {
	var evictedItems []diskKeyAndValue
	now := time.Now().UnixNano()
	c.mu.Lock()
	for k, v := range c.items {
		// "Inlining" of expired
		if v.Expiration > 0 && now > v.Expiration {
			ov, evicted, err := c.delete(k)
			if err != nil {
				return err
			}
			if evicted {
				evictedItems = append(evictedItems, diskKeyAndValue{k, ov})
			}
		}
	}
	c.mu.Unlock()
	for _, v := range evictedItems {
		c.onEvicted(v.key, v.value)
	}
	return nil
}

// Sets an (optional) function that is called with the key and value when an
// item is evicted from the cache. (Including when it is deleted manually, but
// not when it is overwritten.) Set to nil to disable.
func (c *diskCache) OnEvicted(f func(string, interface{})) {
	c.mu.Lock()
	c.onEvicted = f
	c.mu.Unlock()
}

// Write the cache's items (using Gob) to an io.Writer.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *diskCache) Save(w io.Writer) (err error) {
	enc := gob.NewEncoder(w)
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("Error registering item types with Gob library")
		}
	}()
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, v := range c.items {
		gob.Register(v.Object)
	}
	err = enc.Encode(&c.items)
	return
}

// Save the cache's items to the given filename, creating the file if it
// doesn't exist, and overwriting it if it does.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *diskCache) SaveFile(fname string) error {
	fp, err := os.Create(fname)
	if err != nil {
		return err
	}
	err = c.Save(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Add (Gob-serialized) cache items from an io.Reader, excluding any items with
// keys that already exist (and haven't expired) in the current cache.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *diskCache) Load(r io.Reader) error {
	dec := gob.NewDecoder(r)
	items := map[string]*diskItem{}
	err := dec.Decode(&items)
	if err == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		for k, v := range items {
			ov, found := c.items[k]
			if !found || ov.expired() {
				c.items[k] = v
			}
		}
	}
	return err
}

// Load and add cache items from the given filename, excluding any items with
// keys that already exist in the current cache.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *diskCache) LoadFile(fname string) error {
	fp, err := os.Open(fname)
	if err != nil {
		return err
	}
	err = c.Load(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Copies all unexpired items in the cache into a new map and returns it.
func (c *diskCache) Items() map[string]*diskItem {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := make(map[string]*diskItem, len(c.items))
	now := time.Now().UnixNano()
	for k, v := range c.items {
		// "Inlining" of Expired
		if v.Expiration > 0 {
			if now > v.Expiration {
				continue
			}
		}
		m[k] = v
	}
	return m
}

// Returns the number of items in the cache. This may include items that have
// expired, but have not yet been cleaned up.
func (c *diskCache) ItemCount() int {
	c.mu.RLock()
	n := len(c.items)
	c.mu.RUnlock()
	return n
}

// Delete all items from the cache.
func (c *diskCache) Flush() {
	c.mu.Lock()
	c.items = map[string]*diskItem{}
	c.mu.Unlock()
}

type diskJanitor struct {
	interval time.Duration
	stop     chan bool
}

func (j *diskJanitor) Run(c *diskCache) {
	ticker := time.NewTicker(j.interval)
	for {
		select {
		case <-ticker.C:
			c.DeleteExpired()
		case <-j.stop:
			ticker.Stop()
			return
		}
	}
}

func stopDiskJanitor(c *DiskCache) {
	c.janitor.stop <- true
}

func runDiskJanitor(c *diskCache, ci time.Duration) {
	j := &diskJanitor{
		interval: ci,
		stop:     make(chan bool),
	}
	c.janitor = j
	go j.Run(c)
}

func newCache(root string, de time.Duration, m map[string]*diskItem) *diskCache {
	cacheRoot = root + "/.cache/"
	if _, err := os.Stat(cacheRoot); os.IsNotExist(err) {
		os.MkdirAll(cacheRoot, 0700)
	}

	if de == 0 {
		de = -1
	}
	c := &diskCache{
		root:              root,
		defaultExpiration: de,
		items:             m,
	}
	return c
}

func newCacheWithJanitor(root string, de time.Duration, ci time.Duration, m map[string]*diskItem) *DiskCache {
	c := newCache(root, de, m)
	// This trick ensures that the janitor goroutine (which--granted it
	// was enabled--is running DeleteExpired on c forever) does not keep
	// the returned C object from being garbage collected. When it is
	// garbage collected, the finalizer stops the janitor goroutine, after
	// which c can be collected.
	C := &DiskCache{c}
	C.Register(map[string]interface{}{})
	if ci > 0 {
		runDiskJanitor(c, ci)
		runtime.SetFinalizer(C, stopDiskJanitor)
	}
	return C
}

// Return a new cache with a given default expiration duration and cleanup
// interval. If the expiration duration is less than one (or NoExpiration),
// the items in the cache never expire (by default), and must be deleted
// manually. If the cleanup interval is less than one, expired items are not
// deleted from the cache before calling c.DeleteExpired().
func NewDiskCache(root string, defaultExpiration, cleanupInterval time.Duration) *DiskCache {
	items := make(map[string]*diskItem)
	return newCacheWithJanitor(root, defaultExpiration, cleanupInterval, items)
}

// Return a new cache with a given default expiration duration and cleanup
// interval. If the expiration duration is less than one (or NoExpiration),
// the items in the cache never expire (by default), and must be deleted
// manually. If the cleanup interval is less than one, expired items are not
// deleted from the cache before calling c.DeleteExpired().
//
// NewFrom() also accepts an items map which will serve as the underlying map
// for the cache. This is useful for starting from a deserialized cache
// (serialized using e.g. gob.Encode() on c.Items()), or passing in e.g.
// make(map[string]Item, 500) to improve startup performance when the cache
// is expected to reach a certain minimum size.
//
// Only the cache's methods synchronize access to this map, so it is not
// recommended to keep any references to the map around after creating a cache.
// If need be, the map can be accessed at a later point using c.Items() (subject
// to the same caveat.)
//
// Note regarding serialization: When using e.g. gob, make sure to
// gob.Register() the individual types stored in the cache before encoding a
// map retrieved with c.Items(), and to register those same types before
// decoding a blob containing an items map.
func NewDiskCacheFrom(root string, defaultExpiration, cleanupInterval time.Duration, items map[string]*diskItem) *DiskCache {
	return newCacheWithJanitor(root, defaultExpiration, cleanupInterval, items)
}
