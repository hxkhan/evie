package fields

import (
	"strings"
	"sync"
)

type ID int

var (
	mu        sync.RWMutex
	registryA = map[string]ID{}
	registryB = map[ID]string{}
)

func Get(name string) ID {
	mu.RLock()
	index, exists := registryA[name]
	mu.RUnlock()
	if exists {
		return index
	}

	mu.Lock()
	defer mu.Unlock()
	index, exists = registryA[name]
	if !exists {
		// the table lives as long as the process; keep a copy, not a string that
		// may share memory with a whole source file or a buffer the caller reuses
		name = strings.Clone(name)
		id := ID(len(registryA))
		registryA[name] = id
		registryB[id] = name
		return id
	}
	return index
}

// Find returns the id of a name that was interned before, without interning it
func Find(name string) (ID, bool) {
	mu.RLock()
	id, exists := registryA[name]
	mu.RUnlock()
	return id, exists
}

func Lookup(id ID) string {
	mu.RLock()
	defer mu.RUnlock()
	return registryB[id]
}
