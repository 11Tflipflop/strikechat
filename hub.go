package main

import (
	"sync"
	"time"
)

// one chat message
type Message struct {
	ID   int64  `json:"id"`
	Time int64  `json:"time"` // unix ms
	Name string `json:"name"`
	Text string `json:"text"`
}

// Hub keeps the last N messages in memory and sends new ones
// to everyone who is connected. Nothing is written to disk.
type Hub struct {
	mu     sync.Mutex
	nextID int64
	keep   int
	msgs   []Message
	subs   map[chan Message]struct{}
}

func NewHub(keep int) *Hub {
	return &Hub{
		// start ids from the clock so they still go up after a server restart
		nextID: time.Now().UnixMilli(),
		keep:   keep,
		subs:   make(map[chan Message]struct{}),
	}
}

func (h *Hub) Add(name, text string) Message {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextID++
	m := Message{ID: h.nextID, Time: time.Now().UnixMilli(), Name: name, Text: text}

	h.msgs = append(h.msgs, m)
	if len(h.msgs) > h.keep {
		// copy so the old big array can be freed
		h.msgs = append([]Message(nil), h.msgs[len(h.msgs)-h.keep:]...)
	}

	for ch := range h.subs {
		select {
		case ch <- m:
		default:
			// this client is too slow, cut it off.
			// the browser reconnects by itself and gets what it missed.
			delete(h.subs, ch)
			close(ch)
		}
	}
	return m
}

// Subscribe gives back the messages after lastID and a channel for new ones.
// Both happen under one lock so no message can slip in between.
func (h *Hub) Subscribe(lastID int64) ([]Message, chan Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var backlog []Message
	for _, m := range h.msgs {
		if m.ID > lastID {
			backlog = append(backlog, m)
		}
	}
	ch := make(chan Message, 32)
	h.subs[ch] = struct{}{}
	return backlog, ch
}

func (h *Hub) Unsubscribe(ch chan Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *Hub) Online() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
