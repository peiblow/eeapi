package config

import "sync"

type ContractLocker struct {
	locks map[string]*sync.Mutex
	mu    sync.Mutex
}

func NewContractLocker() *ContractLocker {
	return &ContractLocker{
		locks: make(map[string]*sync.Mutex),
	}
}

func (cl *ContractLocker) Lock(contractID string) {
	cl.mu.Lock()
	lock, exists := cl.locks[contractID]
	if !exists {
		lock = &sync.Mutex{}
		cl.locks[contractID] = lock
	}
	cl.mu.Unlock()

	lock.Lock()
}

func (cl *ContractLocker) Unlock(contractID string) {
	cl.mu.Lock()
	lock := cl.locks[contractID]
	cl.mu.Unlock()

	if lock != nil {
		lock.Unlock()
	}
}
