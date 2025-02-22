package ssz

import (
	"reflect"
	"sync"
)

// The marshaler/unmarshaler types take in a value, an output buffer, and a start offset,
// it returns the index of the last byte written and an error, if any.
type marshaler func(reflect.Value, []byte, uint64) (uint64, error)

type unmarshaler func([]byte, reflect.Value, uint64) (uint64, error)

type hasher func(reflect.Value, uint64) ([32]byte, error)

type sszUtils struct {
	marshaler
	unmarshaler
	hasher
}

var (
	sszUtilsCacheMutex sync.RWMutex
	sszUtilsCache      = make(map[reflect.Type]*sszUtils)
	hashCache          = newHashCache(100000)
)

// Get cached encoder, encodeSizer and unmarshaler implementation for a specified type.
// With a cache we can achieve O(1) amortized time overhead for creating encoder, encodeSizer and decoder.
func cachedSSZUtils(typ reflect.Type) (*sszUtils, error) {
	sszUtilsCacheMutex.RLock()
	utils := sszUtilsCache[typ]
	sszUtilsCacheMutex.RUnlock()
	if utils != nil {
		return utils, nil
	}

	// If not found in cache, will get a new one and put it into the cache
	sszUtilsCacheMutex.Lock()
	defer sszUtilsCacheMutex.Unlock()
	return cachedSSZUtilsNoAcquireLock(typ)
}

// This version is used when the caller is already holding the rw lock for sszUtilsCache.
// It doesn't acquire new rw lock so it's free to recursively call itself without getting into
// a deadlock situation.
//
// Make sure you are
func cachedSSZUtilsNoAcquireLock(typ reflect.Type) (*sszUtils, error) {
	// Check again in case other goroutine has just acquired the lock
	// and already updated the cache
	utils := sszUtilsCache[typ]
	if utils != nil {
		return utils, nil
	}
	// Put a dummy value into the cache before generating.
	// If the generator tries to lookup the type of itself,
	// it will get the dummy value and won't call recursively forever.
	sszUtilsCache[typ] = new(sszUtils)
	utils, err := generateSSZUtilsForType(typ)
	if err != nil {
		// Don't forget to remove the dummy key when fail
		delete(sszUtilsCache, typ)
		return nil, err
	}
	// Overwrite the dummy value with real value
	*sszUtilsCache[typ] = *utils
	return sszUtilsCache[typ], nil
}

func generateSSZUtilsForType(typ reflect.Type) (utils *sszUtils, err error) {
	utils = new(sszUtils)
	if utils.marshaler, err = makeMarshaler(typ); err != nil {
		return nil, err
	}
	if utils.unmarshaler, err = makeUnmarshaler(typ); err != nil {
		return nil, err
	}
	if utils.hasher, err = makeHasher(typ); err != nil {
		return nil, err
	}
	return utils, nil
}
