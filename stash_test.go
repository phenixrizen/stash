package stash

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type StashTestSuite struct {
	suite.Suite
	root string
	st   *Stash
}

func (suite *StashTestSuite) SetupTest() {
	tmpDir, err := ioutil.TempDir("", "stash-test")
	suite.Nil(err)
	suite.root = tmpDir
	st, err := New(suite.root, 1<<30, 1*time.Minute, true)
	suite.Nil(err)
	suite.st = st
}

func (suite *StashTestSuite) AfterTest() {
	os.RemoveAll(suite.root) // clean up
}

func (suite *StashTestSuite) TestStash() {
	start := time.Now()
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("foo%d", i)
		val := "bar"
		err := suite.st.Set(key, val, 1*time.Minute)
		suite.Nil(err)
		var rval string
		err = suite.st.Get(key, &rval)
		suite.Nil(err)
		suite.Equal(val, rval)
		fmt.Println(suite.st.GetMemoryCacheStats())
		for j := 0; j < rand.Intn(5000); j++ {
			err = suite.st.Get(key, &rval)
			suite.Nil(err)
			suite.Equal(val, rval)
			fmt.Println(suite.st.GetMemoryCacheStats())
		}
	}

	// test for miss because of TTL
	diff := time.Since(start)
	sleepTime := (61 * time.Second) - diff
	fmt.Printf("Sleeping: %s\n", sleepTime)
	time.Sleep(sleepTime)
	var rval string
	err := suite.st.Get("foo0", &rval)
	suite.NotNil(err)
	suite.Equal("", rval)
	fmt.Println(suite.st.GetMemoryCacheStats())
}

func (suite *StashTestSuite) TestBadStashParams() {
	st, err := New(suite.root, -1, 1*time.Minute, true)
	suite.NotNil(err)
	suite.Nil(st)
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestStashTestSuite(t *testing.T) {
	suite.Run(t, new(StashTestSuite))
}
