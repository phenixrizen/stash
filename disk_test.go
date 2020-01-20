package stash

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewDiskCache(t *testing.T) {
	assert := assert.New(t)

	dc := NewDiskCache(".", 5*time.Minute, 10*time.Minute)
	assert.NotNil(dc)
	assert.IsType(&DiskCache{}, dc)
}

func TestSetGetDiskCache(t *testing.T) {
	assert := assert.New(t)

	dc := NewDiskCache(".", 1*time.Second, 1*time.Minute)
	assert.NotNil(dc)
	assert.IsType(&DiskCache{}, dc)

	obj := map[string]interface{}{
		"x": "y",
		"a": "b",
		"c": true,
	}

	err := dc.Set("test-key", obj, 3*time.Second)
	assert.Nil(err)

	rObj, err := dc.Get("test-key")
	assert.Nil(err)
	assert.Equal(rObj, obj)

	time.Sleep(2 * time.Second)
	rObj2, err := dc.Get("test-key")
	assert.Nil(err)
	assert.Equal(rObj2, obj)

	time.Sleep(2 * time.Second)
	rObj3, err := dc.Get("test-key")
	assert.Nil(err)
	assert.Nil(rObj3)
}
