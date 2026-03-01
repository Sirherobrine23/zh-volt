package util

import (
	"encoding/json"
	"maps"
	"sync"
)

var (
	_ json.Marshaler   = &SyncMap[int, int]{}
	_ json.Unmarshaler = &SyncMap[int, int]{}
)

type SyncMap[K comparable, V any] struct {
	s    *sync.RWMutex
	data map[K]V
}

func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		s:    &sync.RWMutex{},
		data: map[K]V{},
	}
}

func (sy SyncMap[K, V]) MarshalJSON() ([]byte, error) {
	sy.s.RLock()
	defer sy.s.RUnlock()
	return json.Marshal(sy.data)
}

func (sy SyncMap[K, V]) UnmarshalJSON(data []byte) error {
	sy.s.Lock()
	defer sy.s.Unlock()
	return json.Unmarshal(data, &sy.data)
}

func (sy SyncMap[K, V]) Clone() map[K]V {
	sy.s.RLock()
	defer sy.s.RUnlock()
	return maps.Clone(sy.data)
}

func (sy SyncMap[K, V]) Get(k K) (V, bool) {
	sy.s.RLock()
	defer sy.s.RUnlock()
	n, ok := sy.data[k]
	return n, ok
}

func (sy SyncMap[K, V]) Set(k K, v V) {
	sy.s.Lock()
	defer sy.s.Unlock()
	sy.data[k] = v
}

func (sy SyncMap[K, V]) Del(k K) {
	sy.s.Lock()
	defer sy.s.Unlock()
	delete(sy.data, k)
}

func (sy SyncMap[K, V]) Clear() {
	sy.s.Lock()
	defer sy.s.Unlock()
	for key := range sy.data {
		delete(sy.data, key)
	}
}
