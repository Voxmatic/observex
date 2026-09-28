package main

// S1-06 WebSocket hub tests: org-scoped delivery (D5), topic filtering and
// concurrent subscription changes (run with -race where cgo is available).

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func newTestWSClient(orgID string, topics ...string) *wsClient {
	c := &wsClient{orgID: orgID, send: make(chan []byte, 64), topics: map[string]bool{}}
	for _, tp := range topics {
		c.subscribe(tp)
	}
	return c
}

func TestWSShouldDeliver(t *testing.T) {
	tests := []struct {
		name   string
		client *wsClient
		ev     wsEvent
		want   bool
	}{
		{"SameOrg", newTestWSClient("org-a"), wsEvent{Type: "alert", OrgID: "org-a"}, true},
		{"OtherOrg", newTestWSClient("org-a"), wsEvent{Type: "alert", OrgID: "org-b"}, false},
		{"OrgLessEventNotDelivered", newTestWSClient("org-a"), wsEvent{Type: "deployment_result", OrgID: ""}, false},
		{"ClientWithoutOrgGetsNothing", newTestWSClient(""), wsEvent{Type: "alert", OrgID: "org-a"}, false},
		{"ClientWithoutOrg_OrgLessEvent", newTestWSClient(""), wsEvent{Type: "alert", OrgID: ""}, false},
		{"SubscribedTopic", newTestWSClient("org-a", "alert"), wsEvent{Type: "alert", OrgID: "org-a"}, true},
		{"UnsubscribedTopic", newTestWSClient("org-a", "alert"), wsEvent{Type: "deployment", OrgID: "org-a"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wsShouldDeliver(tt.client, tt.ev); got != tt.want {
				t.Fatalf("wsShouldDeliver = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWSClient_UnsubscribeRestoresAllTopics(t *testing.T) {
	c := newTestWSClient("org-a", "alert")
	if c.wants("deployment") {
		t.Fatal("subscribed client should not want other topics")
	}
	c.unsubscribe("alert")
	if !c.wants("deployment") {
		t.Fatal("client with no subscriptions should want every topic")
	}
}

func TestWSHub_DeliversOnlyToSameOrg(t *testing.T) {
	hub := NewWSHub()
	go hub.Run()
	a1 := newTestWSClient("org-a")
	a2 := newTestWSClient("org-a", "alert")
	b := newTestWSClient("org-b")
	for _, c := range []*wsClient{a1, a2, b} {
		hub.register <- c
	}
	waitForClients(t, hub, 3)

	hub.Publish("deployment_result", "", map[string]string{"id": "x"}) // org-less: nobody
	hub.Publish("deployment", "org-a", map[string]string{"id": "y"})   // a1 only (a2 wants alert)
	hub.Publish("alert", "org-a", map[string]string{"id": "z"})        // a1 and a2
	hub.Publish("alert", "org-b", map[string]string{"id": "w"})        // b only

	expect := func(name string, c *wsClient, want []string) {
		t.Helper()
		for _, wantType := range want {
			select {
			case msg := <-c.send:
				var ev wsEvent
				if err := json.Unmarshal(msg, &ev); err != nil || ev.Type != wantType || ev.OrgID != c.orgID {
					t.Fatalf("%s: got %s, want %s for %s", name, msg, wantType, c.orgID)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s: timed out waiting for %s", name, wantType)
			}
		}
		select {
		case msg := <-c.send:
			t.Fatalf("%s: unexpected extra event %s", name, msg)
		case <-time.After(150 * time.Millisecond):
		}
	}
	expect("a1", a1, []string{"deployment", "alert"})
	expect("a2", a2, []string{"alert"})
	expect("b", b, []string{"alert"})
}

func TestWSHub_ConcurrentSubscriptionChanges(t *testing.T) {
	hub := NewWSHub()
	go hub.Run()
	c := newTestWSClient("org-a")
	hub.register <- c
	waitForClients(t, hub, 1)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // read pump behaviour: subscription changes
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			topic := fmt.Sprintf("t%d", i%5)
			c.subscribe(topic)
			c.unsubscribe(topic)
		}
	}()
	go func() { // drain deliveries
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case <-c.send:
			}
		}
	}()
	for i := 0; i < 2000; i++ {
		hub.Publish(fmt.Sprintf("t%d", i%5), "org-a", i)
	}
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
