package db

import (
	"ignite/config"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

var Bucket = config.Defaults.DB.Bucket

func TestBoltKV(t *testing.T) {
	originalDBPath := config.Defaults.DB.DBPath
	originalDBFile := config.Defaults.DB.DBFile
	config.Defaults.DB.DBPath = "../"
	config.Defaults.DB.DBFile = "test.db"
	defer func() {
		os.Remove(config.Defaults.DB.DBPath + config.Defaults.DB.DBFile)
		config.Defaults.DB.DBPath = originalDBPath
		config.Defaults.DB.DBFile = originalDBFile
	}()

	kv, err := Init()
	if !assert.NoError(t, err) {
		return
	}
	defer kv.Close()

	t.Run("PutKV and GetKV", func(t *testing.T) {
		key := []byte("key1")
		value := []byte("value1")
		err := kv.PutKV(Bucket, key, value)
		assert.NoError(t, err)

		val, err := kv.GetKV(Bucket, key)
		assert.NoError(t, err)
		assert.Equal(t, value, val)
	})

	t.Run("DeleteKV", func(t *testing.T) {
		key := []byte("keyToDelete")
		value := []byte("valueToDelete")
		err := kv.PutKV(Bucket, key, value)
		assert.NoError(t, err)

		err = kv.DeleteKV(Bucket, key)
		assert.NoError(t, err)

		val, err := kv.GetKV(Bucket, key)
		assert.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("GetAllKV", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			err := kv.PutKV(Bucket, []byte(strconv.Itoa(i)), []byte("test-value-"+strconv.Itoa(i)))
			assert.NoError(t, err)
		}

		allKV, err := kv.GetAllKV(Bucket)
		assert.NoError(t, err)
		assert.Len(t, allKV, 4)

		for i := 0; i < 3; i++ {
			keyStr := strconv.Itoa(i)
			assert.Equal(t, []byte("test-value-"+keyStr), allKV[keyStr])
		}
	})

	t.Run("DeleteAllKV", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			err := kv.PutKV(Bucket, []byte(strconv.Itoa(i)), []byte("test-value-"+strconv.Itoa(i)))
			assert.NoError(t, err)
		}

		err := kv.DeleteAllKV(Bucket)
		assert.NoError(t, err)

		allKV, err := kv.GetAllKV(Bucket)
		assert.NoError(t, err)
		assert.Empty(t, allKV)
	})
}
