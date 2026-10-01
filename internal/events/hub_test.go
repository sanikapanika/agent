package events

import "testing"

func TestCloseEndsSubscriptions(t *testing.T) {
	h := NewHub()
	ch, unsubscribe := h.Subscribe()
	h.Publish(Message{Type: "result"})
	if m := <-ch; m.Type != "result" {
		t.Fatalf("got %+v", m)
	}
	h.Close()
	if _, ok := <-ch; ok {
		t.Fatal("channel still open after Close")
	}
	unsubscribe() // must not panic on an already-closed channel
	late, _ := h.Subscribe()
	if _, ok := <-late; ok {
		t.Fatal("subscribing after Close should return a closed channel")
	}
	h.Publish(Message{}) // must not panic
}
