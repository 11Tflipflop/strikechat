package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	s := NewServer("test", "secret", NewHub(50), []byte("<html></html>"), 10)
	ts := httptest.NewServer(s.Routes())
	t.Cleanup(ts.Close)
	return ts, s
}

func post(t *testing.T, url, body string) int {
	t.Helper()
	r, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return r.StatusCode
}

func waitOnline(t *testing.T, s *Server, n int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if s.hub.Online() == n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected %d online, got %d", n, s.hub.Online())
}

func TestBadKeyRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	r, _ := http.Get(ts.URL + "/api/info?k=wrong")
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", r.StatusCode)
	}
	r, _ = http.Get(ts.URL + "/api/info")
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing key: want 401, got %d", r.StatusCode)
	}
}

func TestIndexNeedsNoKey(t *testing.T) {
	ts, _ := newTestServer(t)
	r, _ := http.Get(ts.URL + "/")
	if r.StatusCode != 200 {
		t.Fatalf("index: got %d", r.StatusCode)
	}
}

func TestSendValidation(t *testing.T) {
	ts, _ := newTestServer(t)
	u := ts.URL + "/api/send?k=secret"
	if c := post(t, u, `{"name":"a","text":""}`); c != 400 {
		t.Fatalf("empty text: got %d", c)
	}
	if c := post(t, u, `not json`); c != 400 {
		t.Fatalf("bad json: got %d", c)
	}
	if c := post(t, u, `{"name":"a","text":"hello"}`); c != 204 {
		t.Fatalf("good message: got %d", c)
	}
}

func TestSendRateLimit(t *testing.T) {
	ts, _ := newTestServer(t)
	u := ts.URL + "/api/send?k=secret"
	got429 := false
	for i := 0; i < 10; i++ {
		if post(t, u, `{"name":"a","text":"spam"}`) == 429 {
			got429 = true
		}
	}
	if !got429 {
		t.Fatal("rate limit never kicked in")
	}
}

func TestSendThenReceiveOverStream(t *testing.T) {
	ts, srv := newTestServer(t)

	resp, err := http.Get(ts.URL + "/api/stream?k=secret")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type: %q", ct)
	}

	// wait until the server has registered us, then send
	waitOnline(t, srv, 1)
	if c := post(t, ts.URL+"/api/send?k=secret", `{"name":"sam","text":"hello world"}`); c != 204 {
		t.Fatalf("send: %d", c)
	}

	got := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data: ") {
				got <- sc.Text()
				return
			}
		}
	}()
	select {
	case line := <-got:
		if !strings.Contains(line, "hello world") || !strings.Contains(line, "sam") {
			t.Fatalf("unexpected event: %s", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message came over the stream")
	}
}

func TestRoomFull(t *testing.T) {
	s := NewServer("test", "", NewHub(10), []byte("x"), 1)
	ts := httptest.NewServer(s.Routes())
	defer ts.Close()

	r1, err := http.Get(ts.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Body.Close()
	waitOnline(t, s, 1)

	r2, err := http.Get(ts.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second client: want 503, got %d", r2.StatusCode)
	}
}
