package application

import "sync"

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

type Coordinator struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

func NewCoordinator() *Coordinator { return &Coordinator{locks: map[string]*keyedLock{}} }

func (c *Coordinator) WithKey(key string, fn func() error) error {
	c.mu.Lock()
	lock := c.locks[key]
	if lock == nil {
		lock = &keyedLock{}
		c.locks[key] = lock
	}
	lock.refs++
	c.mu.Unlock()
	lock.mu.Lock()
	defer func() {
		lock.mu.Unlock()
		c.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(c.locks, key)
		}
		c.mu.Unlock()
	}()
	return fn()
}
