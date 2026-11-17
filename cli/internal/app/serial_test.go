package app

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/onehumanbeing/esp32-iphone-agent/cli/internal/daemon"
)

func TestSerialCLIAndMCPShareRequests(t *testing.T) {
	dir, err := os.MkdirTemp("", "serial-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	l, err := net.Listen("unix", filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	requests := make(chan daemon.Request, 16)
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				var r daemon.Request
				if json.NewDecoder(c).Decode(&r) != nil {
					return
				}
				requests <- r
				json.NewEncoder(c).Encode(daemon.Response{Status: "ok", Data: map[string]any{"message": "submitted"}})
			}()
		}
	}()
	a := &App{Err: io.Discard, Dir: dir, Client: &daemon.Client{Dir: dir, NoSpawn: true}}
	if r := run(t, a, "io", "scroll", "-1", "--device", "COM24"); r.Status != "ok" {
		t.Fatal(r)
	}
	r := <-requests
	if r.Cmd != "scroll" || r.Text != "-1" || r.Device != "COM24" {
		t.Fatal(r)
	}
	if result := a.callTool("scroll", map[string]any{"amount": float64(4), "device": "COM24"}); result["isError"] == true {
		t.Fatal(result)
	}
	r = <-requests
	if r.Cmd != "scroll" || r.Text != "4" || r.Device != "COM24" {
		t.Fatal(r)
	}
	if result := a.callTool("tap", map[string]any{"x": float64(30), "y": float64(40), "device": "COM24"}); result["isError"] == true {
		t.Fatal(result)
	}
	r = <-requests
	if r.Cmd != "tap" || len(r.Pts) != 2 || r.Pts[0] != 30 || r.Pts[1] != 40 {
		t.Fatal(r)
	}
}

func TestSerialMCPRejectsInvalidBeforeDevice(t *testing.T) {
	a := &App{Err: io.Discard}
	for _, v := range []any{nil, "1", 1.5, float64(5)} {
		if r := a.callTool("scroll", map[string]any{"amount": v}); r["isError"] != true {
			t.Fatal(r)
		}
	}
	for _, args := range []map[string]any{{"x": float64(1)}, {"x": "1", "y": float64(2)}, {"x": float64(1), "y": float64(2), "count": 1.5}} {
		if r := a.callTool("tap", args); r["isError"] != true {
			t.Fatal(r)
		}
	}
	for _, v := range []string{"1.5", "5", "1 2"} {
		if r := run(t, a, "io", "scroll", v); r.Status != "error" {
			t.Fatal(r)
		}
	}
}
