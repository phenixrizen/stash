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

	err := dc.Set("test-key", obj, 5*time.Minute)
	assert.Nil(err)

	var rObj map[string]interface{}
	ok, err := dc.Get("test-key", rObj)
	assert.Nil(err)
	assert.True(ok)
	assert.Equal(rObj, obj)

	time.Sleep(2 * time.Second)
	ok, err = dc.Get("test-key", rObj)
	assert.Nil(err)
	assert.True(ok)
	assert.Equal(rObj, obj)
}
