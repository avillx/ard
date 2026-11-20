package chat

import (
	"ard/internal/session"
	"sync"
	"time"
)

const DefaultCacheLifeTime = 30 * time.Minute

type Cache struct {
	chats        map[session.ID]*Chat
	deleteTimers map[session.ID]*time.Timer
	ttl          time.Duration

	mu sync.Mutex
}

func NewCache(cacheTTL time.Duration) *Cache {
	return &Cache{
		chats:        map[session.ID]*Chat{},
		deleteTimers: map[session.ID]*time.Timer{},
		ttl:          cacheTTL,
	}
}

func (c *Cache) Put(sessID session.ID, chats *Chat) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// try to delete
	delete(c.chats, sessID)
	if t, ok := c.deleteTimers[sessID]; ok {
		t.Stop()
		delete(c.deleteTimers, sessID)
	}

	c.chats[sessID] = chats
	c.deleteTimers[sessID] = time.AfterFunc(c.ttl, func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		delete(c.chats, sessID)
		delete(c.deleteTimers, sessID)
	})
}

func (c *Cache) Get(sessID session.ID) (*Chat, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg, ok := c.chats[sessID]

	if ok {
		c.deleteTimers[sessID].Reset(c.ttl)
	}

	return msg, ok
}
