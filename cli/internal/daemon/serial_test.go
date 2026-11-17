package daemon

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type testSerial struct {
	mu    sync.Mutex
	lines []string
	bad   string
	up    int
}

func (f *testSerial) Send(ctx context.Context, cmd string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, cmd)
	if cmd == f.bad {
		return "", errors.New("outcome unknown; not replayed")
	}
	if cmd == "status" {
		return fmt.Sprintf(`{"fw":"0.2.5","usb":"ok","abs":1,"up":%d}`, f.up), nil
	}
	return "", nil
}
func (f *testSerial) Close() error { return nil }
func (f *testSerial) commands() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.lines, "|")
}

func serialServer(t *testing.T) (*Server, *Conn, *testSerial) {
	t.Helper()
	f := &testSerial{up: 1000}
	s := &Server{Dir: t.TempDir()}
	c := newConn("COMtest", "test", "")
	c.serial = f
	c.actionGate = make(chan struct{}, 1)
	c.actionGate <- struct{}{}
	c.frame = image.NewRGBA(image.Rect(0, 0, 496, 1080))
	c.captured = time.Now()
	c.status = map[string]any{}
	s.conns = map[string]*Conn{"COMtest": c}
	return s, c, f
}
func TestSerialTapUsesAbsoluteNoHome(t *testing.T) {
	s, _, f := serialServer(t)
	r := s.Do(context.Background(), Request{Cmd: "tap", Device: "COMtest", Pts: []float64{247.5, 539.5}})
	if r.Status != "ok" {
		t.Fatal(r)
	}
	if got := f.commands(); got != "status|abs 16384 16384|click" {
		t.Fatal(got)
	}
	if r.Data.(map[string]any)["effect_verified"] != false {
		t.Fatal(r)
	}
}
func TestSerialScrollRejectsAndMaps(t *testing.T) {
	for _, v := range []string{"-4", "0", "1", "4", "1.5", "5", "1 2"} {
		t.Run(v, func(t *testing.T) {
			s, _, f := serialServer(t)
			r := s.Do(context.Background(), Request{Cmd: "scroll", Device: "COMtest", Text: v})
			invalid := v == "1.5" || v == "5" || v == "1 2"
			if invalid {
				if r.Status != "error" || strings.Contains(f.commands(), "scroll_by") {
					t.Fatal(r, f.commands())
				}
			} else if r.Status != "ok" || !strings.HasSuffix(f.commands(), "scroll_by "+v) {
				t.Fatal(r, f.commands())
			}
		})
	}
}
func TestSerialCancelledHoldAlwaysReleases(t *testing.T) {
	s, _, f := serialServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	r := s.Do(ctx, Request{Cmd: "longpress", Device: "COMtest", Pts: []float64{100, 100}, Ms: 1000})
	if r.Status != "error" || !strings.HasSuffix(f.commands(), "press_for 1000|reset") {
		t.Fatal(r, f.commands())
	}
}
func TestSerialFailedMoveNeverClicks(t *testing.T) {
	s, _, f := serialServer(t)
	f.bad = "abs 16384 16384"
	r := s.Do(context.Background(), Request{Cmd: "tap", Device: "COMtest", Pts: []float64{247.5, 539.5}})
	if r.Status != "error" || strings.Contains(f.commands(), "click") {
		t.Fatal(r, f.commands())
	}
}
func TestSerialRebootBlocksInputs(t *testing.T) {
	s, c, f := serialServer(t)
	c.status["up"] = float64(2000)
	for i := 0; i < 2; i++ {
		r := s.Do(context.Background(), Request{Cmd: "keys", Device: "COMtest", Text: "esc"})
		if r.Status != "error" {
			t.Fatal(r)
		}
	}
	if strings.Contains(f.commands(), "key") {
		t.Fatal(f.commands())
	}
}
func TestSerialWholeActionOwnsQueue(t *testing.T) {
	s, _, f := serialServer(t)
	done := make(chan Response)
	go func() {
		done <- s.Do(context.Background(), Request{Cmd: "longpress", Device: "COMtest", Pts: []float64{100, 100}, Ms: 100})
	}()
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(f.commands(), "press_for") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	r := s.Do(context.Background(), Request{Cmd: "keys", Device: "COMtest", Text: "esc"})
	if r.Status != "ok" {
		t.Fatal(r)
	}
	if r := <-done; r.Status != "ok" {
		t.Fatal(r)
	}
	if !strings.Contains(f.commands(), "press_for 100|reset|status|key esc") {
		t.Fatal(f.commands())
	}
}
func TestHTTPFramesRequireRealMetadata(t *testing.T) {
	for _, kind := range []string{"fresh", "old", "missing", "empty"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "empty" {
					w.WriteHeader(204)
					return
				}
				if kind != "missing" {
					at := time.Now()
					if kind == "old" {
						at = at.Add(-time.Minute)
					}
					w.Header().Set("X-Frame-Time", fmt.Sprint(float64(at.UnixMilli())/1000))
					w.Header().Set("X-Frame-Id", "capture:1")
				}
				png.Encode(w, image.NewRGBA(image.Rect(0, 0, 12, 12)))
			}))
			defer server.Close()
			img, _, _, err := fetchFrame(context.Background(), server.URL)
			if kind == "fresh" {
				if err != nil || img == nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("accepted invalid frame")
			}
		})
	}
}
