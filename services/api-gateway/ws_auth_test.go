package main

// S1-06 WebSocket authentication tests (R11). /ws accepts only the
// Authorization: Bearer header, the same as REST. Query-string and
// subprotocol tokens are not accepted. JWT values are never printed.

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
)

func startS106Server(t *testing.T) (*Gateway, string) {
	t.Helper()
	gw, app := newS106Gateway(t, "")
	go gw.hub.Run()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = app.Listener(ln) }()
	return gw, ln.Addr().String()
}

func dialWS(t *testing.T, addr, query string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	dialer := &websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	u := "ws://" + addr + "/ws"
	if query != "" {
		u += "?" + query
	}
	return dialer.Dial(u, header)
}

func bearer(t *testing.T, userID string) http.Header {
	return http.Header{"Authorization": []string{"Bearer " + s106JWT(t, userID)}}
}

func readEvent(t *testing.T, conn *websocket.Conn, timeout time.Duration) (wsEvent, error) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return wsEvent{}, err
	}
	var ev wsEvent
	if err := json.Unmarshal(msg, &ev); err != nil {
		t.Fatalf("invalid event JSON: %s", msg)
	}
	return ev, nil
}

func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
}

func TestWS_RejectedHandshakes(t *testing.T) {
	_, addr := startS106Server(t)
	jwtA := s106JWT(t, "editor-a")

	tests := []struct {
		name       string
		query      string
		header     http.Header
		wantStatus int
	}{
		{"NoCredentials_401", "", nil, http.StatusUnauthorized},
		{"InvalidToken_401", "", http.Header{"Authorization": []string{"Bearer not-a-valid-jwt"}}, http.StatusUnauthorized},
		{"QueryStringTokenNotAccepted_401", "token=" + jwtA, nil, http.StatusUnauthorized},
		{"QueryStringAccessTokenNotAccepted_401", "access_token=" + jwtA, nil, http.StatusUnauthorized},
		{"SubprotocolTokenNotAccepted_401", "", http.Header{"Sec-WebSocket-Protocol": []string{"observex.bearer." + jwtA}}, http.StatusUnauthorized},
		{"UserWithoutOrg_403", "", bearer(t, "editor-noorg"), http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, resp, err := dialWS(t, addr, tt.query, tt.header)
			defer closeResp(resp)
			if err == nil {
				conn.Close()
				t.Fatal("handshake succeeded, want rejection")
			}
			if resp == nil {
				t.Fatalf("no HTTP response for rejected handshake: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestWS_NonUpgradeRequest_426(t *testing.T) {
	_, app := newS106Gateway(t, "")
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+s106JWT(t, "editor-a"))
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUpgradeRequired {
		t.Fatalf("status = %d, want 426", resp.StatusCode)
	}
}

func TestWS_ValidBearer_UpgradesWithCallerOrg(t *testing.T) {
	_, addr := startS106Server(t)
	for _, tc := range []struct{ user, org string }{{"viewer-a", "org-a"}, {"editor-b", "org-b"}} {
		t.Run(tc.user, func(t *testing.T) {
			conn, resp, err := dialWS(t, addr, "", bearer(t, tc.user))
			defer closeResp(resp)
			if err != nil {
				status := 0
				if resp != nil {
					status = resp.StatusCode
				}
				t.Fatalf("handshake failed (status %d): %v", status, err)
			}
			defer conn.Close()
			ev, err := readEvent(t, conn, 3*time.Second)
			if err != nil {
				t.Fatalf("read welcome: %v", err)
			}
			if ev.Type != "connected" || ev.OrgID != tc.org {
				t.Fatalf("welcome = %+v, want type connected org %s", ev, tc.org)
			}
		})
	}
}

func TestWS_OrgIsolationAndPong(t *testing.T) {
	gw, addr := startS106Server(t)

	connA, respA, err := dialWS(t, addr, "", bearer(t, "viewer-a"))
	defer closeResp(respA)
	if err != nil {
		t.Fatalf("dial org-a: %v", err)
	}
	defer connA.Close()
	connB, respB, err := dialWS(t, addr, "", bearer(t, "viewer-b"))
	defer closeResp(respB)
	if err != nil {
		t.Fatalf("dial org-b: %v", err)
	}
	defer connB.Close()

	for _, c := range []*websocket.Conn{connA, connB} {
		if ev, err := readEvent(t, c, 3*time.Second); err != nil || ev.Type != "connected" {
			t.Fatalf("welcome: %+v %v", ev, err)
		}
	}
	waitForClients(t, gw.hub, 2)

	// Org-less event first (must reach nobody), then an org-a event.
	gw.hub.Publish("deployment_result", "", map[string]string{"id": "d-1"})
	gw.hub.Publish("s106_test_event", "org-a", map[string]string{"k": "v"})

	ev, err := readEvent(t, connA, 3*time.Second)
	if err != nil {
		t.Fatalf("org-a read: %v", err)
	}
	if ev.Type != "s106_test_event" || ev.OrgID != "org-a" {
		t.Fatalf("org-a received %+v, want s106_test_event for org-a (org-less event must not be delivered)", ev)
	}

	// Pong is queued through the write pump.
	if err := connA.WriteMessage(websocket.TextMessage, []byte(`{"action":"ping"}`)); err != nil {
		t.Fatalf("send ping: %v", err)
	}
	if ev, err := readEvent(t, connA, 3*time.Second); err != nil || ev.Type != "pong" {
		t.Fatalf("pong: %+v %v", ev, err)
	}

	ev, err = readEvent(t, connB, 400*time.Millisecond)
	if err == nil {
		t.Fatalf("org-b received an event it must not see: %+v", ev)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("org-b read: want timeout (no events), got %v", err)
	}
}
