package main

import "testing"

func TestHubBacklogAndKeep(t *testing.T) {
	h := NewHub(3)
	for i := 0; i < 5; i++ {
		h.Add("a", "msg")
	}
	backlog, ch := h.Subscribe(0)
	defer h.Unsubscribe(ch)
	if len(backlog) != 3 {
		t.Fatalf("want 3 kept messages, got %d", len(backlog))
	}
	// ids must go up
	for i := 1; i < len(backlog); i++ {
		if backlog[i].ID <= backlog[i-1].ID {
			t.Fatal("ids not increasing")
		}
	}
}

func TestHubResumeFromLastID(t *testing.T) {
	h := NewHub(10)
	m1 := h.Add("a", "one")
	h.Add("a", "two")
	backlog, ch := h.Subscribe(m1.ID)
	defer h.Unsubscribe(ch)
	if len(backlog) != 1 || backlog[0].Text != "two" {
		t.Fatalf("resume gave wrong backlog: %+v", backlog)
	}
}

func TestHubLiveDelivery(t *testing.T) {
	h := NewHub(10)
	_, ch := h.Subscribe(0)
	defer h.Unsubscribe(ch)
	h.Add("a", "hello")
	m := <-ch
	if m.Text != "hello" {
		t.Fatalf("got %q", m.Text)
	}
}

func TestHubSlowClientIsCutOff(t *testing.T) {
	h := NewHub(100)
	_, ch := h.Subscribe(0) // never read from it
	for i := 0; i < 100; i++ {
		h.Add("a", "spam")
	}
	// channel should have been closed after its buffer filled up
	closed := false
	for range ch {
	}
	closed = true
	if !closed || h.Online() != 0 {
		t.Fatal("slow client was not removed")
	}
}
