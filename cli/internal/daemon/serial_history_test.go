package daemon

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/onehumanbeing/esp32-iphone-agent/cli/internal/history"
)

type timedSerial struct {
	*testSerial
	mu   sync.Mutex
	keys []time.Time
}

func (f *timedSerial) Send(ctx context.Context, cmd string) (string, error) {
	if strings.HasPrefix(cmd, "key ") || strings.HasPrefix(cmd, "type ") {
		f.mu.Lock()
		f.keys = append(f.keys, time.Now())
		f.mu.Unlock()
	}
	return f.testSerial.Send(ctx, cmd)
}

func TestSerialHistoryPinyinAndKeyPacing(t *testing.T) {
	s, c, f := serialServer(t)
	clock := &timedSerial{testSerial: f}
	c.serial = clock
	requests := []Request{
		{Cmd: "tap", Pts: []float64{248, 540}},
		{Cmd: "keys", Text: "Backspace"}, {Cmd: "keys", Text: "backspace"},
		{Cmd: "text", Text: "拿铁"}, {Cmd: "text", Text: "ok"},
		{Cmd: "scroll", Text: "-1"},
		{Cmd: "swipe", Pts: []float64{248, 864, 248, 216}},
	}
	for _, req := range requests {
		req.Device = "COMtest"
		r := s.Do(context.Background(), req)
		if r.Status != "ok" {
			t.Fatal(r)
		}
		if req.Text == "拿铁" && r.Data.(map[string]any)["typed_as"] != "natie" {
			t.Fatal(r)
		}
		if req.Text == "ok" && r.Data.(map[string]any)["typed_as"] != nil {
			t.Fatal(r)
		}
	}
	clock.mu.Lock()
	times := append([]time.Time(nil), clock.keys...)
	clock.mu.Unlock()
	if len(times) != 4 {
		t.Fatal(times)
	}
	for i := 1; i < len(times); i++ {
		if times[i].Sub(times[i-1]) < KeyGap {
			t.Fatal("keyboard pacing lost", times)
		}
	}
	run, err := history.Load(history.Root(s.Dir), "latest")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, step := range run.Steps {
		got = append(got, step.JSON())
	}
	want := []string{`{"action": "tap", "x": 500, "y": 500}`, `{"action": "key", "combo": "backspace"}`, `{"action": "key", "combo": "backspace"}`, `{"action": "type", "text": "拿铁"}`, `{"action": "type", "text": "ok"}`, `{"action": "scroll", "amount": -1}`}
	want = append(want, `{"action": "swipe", "x1": 500, "x2": 500, "y1": 800, "y2": 200}`)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatal(got)
	}
	if strings.Contains(f.commands(), "home") {
		t.Fatal("relative homing introduced", f.commands())
	}
}

func TestSerialOwnerAndUnsupportedPhoneAudio(t *testing.T) {
	s, _, f := dualFixture(t)
	f.state["owner"] = "another-controller"
	for _, req := range []Request{{Cmd: "keys", Text: "esc"}, {Cmd: "send", Text: "click"}} {
		req.Device = "COMtest"
		r := s.Do(context.Background(), req)
		if r.Status != "error" || !strings.Contains(r.Error, "owns") {
			t.Fatal(r)
		}
	}
	if strings.Join(f.lines, "|") != "status|status" {
		t.Fatal(f.lines)
	}
	f.lines = nil
	for _, req := range []Request{{Cmd: "listen", Text: "on"}, {Cmd: "button", Text: "VOLUME_UP"}} {
		req.Device = "COMtest"
		if r := s.Do(context.Background(), req); r.Status != "error" {
			t.Fatal(r)
		}
	}
	for _, cmd := range f.lines {
		if cmd != "status" {
			t.Fatal("unsupported command wrote input", f.lines)
		}
	}
}
