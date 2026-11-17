package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

type dualSerial struct {
	mu     sync.Mutex
	state  map[string]any
	lines  []string
	closed bool
}

func (f *dualSerial) Send(_ context.Context, cmd string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, cmd)
	if cmd == "status" {
		b, e := json.Marshal(f.state)
		return string(b), e
	}
	if strings.HasPrefix(cmd, "output ") {
		f.state["hid_output"] = strings.TrimPrefix(cmd, "output ")
	}
	return "", nil
}
func (f *dualSerial) Close() error { f.closed = true; return nil }
func dualFixture(t *testing.T) (*Server, *Conn, *dualSerial) {
	t.Helper()
	s, c, _ := serialServer(t)
	f := &dualSerial{state: map[string]any{
		"fw": "0.3.0-serial", "usb": "off", "hid_output": "ble", "abs": 1, "up": 1000,
		"device_id": "board-a", "boot_id": "boot-a", "connection_generation": 1,
		"ble_ready": true, "ble_connected": true, "ble_encrypted": true, "ready": true, "fault": false,
	}}
	c.serial = f
	c.hidOutput = "ble"
	return s, c, f
}
func TestBLEDoesNotRequireUSBAndChunksText(t *testing.T) {
	s, _, f := dualFixture(t)
	r := s.Do(context.Background(), Request{Cmd: "text", Device: "COMtest", Text: "12345678901234567890"})
	if r.Status != "ok" || r.Data.(map[string]any)["hid_output"] != "ble" {
		t.Fatal(r)
	}
	got := strings.Join(f.lines, "|")
	if !strings.Contains(got, "type 1234567890123456|type 7890") {
		t.Fatal(got)
	}
}
func TestBLEReadinessCannotBorrowUSB(t *testing.T) {
	for _, key := range []string{"ble_ready", "ble_connected", "ble_encrypted", "ready", "fault"} {
		t.Run(key, func(t *testing.T) {
			s, _, f := dualFixture(t)
			f.state["usb"] = "ok"
			f.state[key] = key == "fault"
			for _, cmd := range []string{"keys", "send"} {
				r := s.Do(context.Background(), Request{Cmd: cmd, Device: "COMtest", Text: "key esc"})
				if r.Status != "error" {
					t.Fatal(r)
				}
			}
			if strings.Join(f.lines, "|") != "status|status" {
				t.Fatal(f.lines)
			}
		})
	}
}
func TestBLESessionChangesLatchUntilRestart(t *testing.T) {
	for _, key := range []string{"device_id", "boot_id", "connection_generation", "hid_output", "fault"} {
		t.Run(key, func(t *testing.T) {
			s, c, f := dualFixture(t)
			if err := c.refreshSerial(context.Background()); err != nil {
				t.Fatal(err)
			}
			old := f.state[key]
			switch key {
			case "connection_generation":
				f.state[key] = 2
			case "hid_output":
				f.state[key] = "usb"
			case "fault":
				f.state[key] = true
			default:
				f.state[key] = "changed"
			}
			r := s.Do(context.Background(), Request{Cmd: "keys", Device: "COMtest", Text: "esc"})
			if r.Status != "error" {
				t.Fatal(r)
			}
			f.state[key] = old
			r = s.Do(context.Background(), Request{Cmd: "keys", Device: "COMtest", Text: "esc"})
			if r.Status != "error" || strings.Contains(strings.Join(f.lines, "|"), "key ") {
				t.Fatal(r, f.lines)
			}
		})
	}
}
func TestSerialOutputSelection(t *testing.T) {
	for _, mode := range []string{"usb", "ble", "invalid", "legacy-ble"} {
		t.Run(mode, func(t *testing.T) {
			_, _, f := dualFixture(t)
			f.state["hid_output"] = "usb"
			f.state["usb"] = "ok"
			f.state["output_select"] = true
			env := mode
			if mode == "legacy-ble" {
				env = "ble"
				delete(f.state, "output_select")
				delete(f.state, "hid_output")
			}
			t.Setenv("ESP32AGENT_HID_OUTPUT", env)
			s := &Server{OpenSerial: func(string) (commandLink, error) { return f, nil }}
			c, err := s.serialConn(context.Background(), "COMtest")
			if mode == "invalid" || mode == "legacy-ble" {
				if err == nil {
					t.Fatal("accepted invalid configuration")
				}
				if strings.Contains(strings.Join(f.lines, "|"), "output ") {
					t.Fatal(f.lines)
				}
				if mode == "invalid" && len(f.lines) != 0 {
					t.Fatal(f.lines)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer c.close()
			if c.hidOutput != mode || !strings.Contains(strings.Join(f.lines, "|"), "output "+mode) {
				t.Fatal(c.hidOutput, f.lines)
			}
		})
	}
}
