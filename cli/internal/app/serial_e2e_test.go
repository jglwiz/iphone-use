package app

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/onehumanbeing/esp32-iphone-agent/cli/internal/daemon"
	"github.com/onehumanbeing/esp32-iphone-agent/cli/internal/seriallink"
)

// This fixture exercises the real daemon socket and seriallink byte protocol.
// It deliberately does not implement the removed PC Bluetooth/Wi-Fi protocol.
type acceptanceBoard struct {
	mu       sync.Mutex
	commands []string
	state    map[string]any
	opens    int
}

func (b *acceptanceBoard) open(port string) (daemon.CommandLink, error) {
	if port != "COMfixture" {
		return nil, fmt.Errorf("unexpected port %q", port)
	}
	b.mu.Lock()
	b.opens++
	b.mu.Unlock()
	pc, board := net.Pipe()
	go func() {
		defer board.Close()
		r := bufio.NewScanner(board)
		for r.Scan() {
			id, command, found := strings.Cut(r.Text(), " ")
			if !found {
				return
			}
			b.mu.Lock()
			b.commands = append(b.commands, command)
			reply := "ok"
			if command == "status" {
				raw, _ := json.Marshal(b.state)
				reply = string(raw)
			} else if strings.HasPrefix(command, "output ") {
				b.state["hid_output"] = strings.TrimPrefix(command, "output ")
			} else if command == "scroll_by 4" {
				reply = "err test rejection"
			} else if command == "crash" || strings.HasPrefix(command, "logs ") {
				reply = "err unknown command"
			}
			b.mu.Unlock()
			// Fragment replies, interleave boot/debug and mismatched IDs.
			wire := "boot diagnostic\n17 ok unrelated\n" + id + " " + reply + "\r\n"
			for _, part := range []byte(wire) {
				if _, err := board.Write([]byte{part}); err != nil {
					return
				}
			}
		}
	}()
	return seriallink.New(pc), nil
}

func (b *acceptanceBoard) snapshot() (string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.commands, "|"), b.opens
}

func TestSerialCLIAndMCPEndToEnd(t *testing.T) {
	for _, output := range []string{"usb", "ble"} {
		t.Run(output, func(t *testing.T) {
			t.Setenv("ESP32AGENT_PORT", "COMfixture")
			t.Setenv("ESP32AGENT_HID_OUTPUT", output)
			frames := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Frame-Time", fmt.Sprint(float64(time.Now().UnixMilli())/1000))
				w.Header().Set("X-Frame-Id", fmt.Sprint(time.Now().UnixNano()))
				png.Encode(w, image.NewRGBA(image.Rect(0, 0, 496, 1080)))
			}))
			defer frames.Close()
			t.Setenv("ESP32AGENT_FRAME_URL", frames.URL)
			board := &acceptanceBoard{state: map[string]any{"fw": "0.3.0-serial", "usb": "ok", "abs": 1,
				"hid_output": "usb", "output_select": true, "device_id": "fixture", "boot_id": "boot1",
				"connection_generation": 1, "ble_ready": true, "ble_connected": true, "ble_encrypted": true, "ready": true}}
			if output == "ble" {
				board.state["usb"] = "off"
			}
			// Short socket path also works on Windows AF_UNIX.
			dir, err := os.MkdirTemp("", "hid-e2e-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			listener, err := net.Listen("unix", filepath.Join(dir, "daemon.sock"))
			if err != nil {
				t.Fatal(err)
			}
			server := &daemon.Server{Dir: dir, OpenSerial: board.open}
			done := make(chan error, 1)
			go func() { done <- server.Serve(listener) }()
			defer func() { server.Stop(); <-done }()
			a := &App{Err: io.Discard, Dir: dir, Client: &daemon.Client{Dir: dir, NoSpawn: true}}
			if r := run(t, a, "device", "info"); r.Status != "ok" || r.Data["hid_output"] != output || r.Data["control_transport"] != "serial" {
				t.Fatal(r)
			}
			mcp := func(name string, args map[string]any) map[string]any {
				t.Helper()
				request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
				var out bytes.Buffer
				a.Out = &out
				a.mcp(strings.NewReader(string(request) + "\n"))
				var response struct{ Result map[string]any }
				if err := json.Unmarshal(out.Bytes(), &response); err != nil {
					t.Fatal(err, out.String())
				}
				return response.Result
			}
			if r := run(t, a, "screenshot", "--output", filepath.Join(dir, "screen.png")); r.Status != "ok" {
				t.Fatal(r)
			} else {
				file, err := os.Open(r.Data["filePath"].(string))
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.Decode(file)
				file.Close()
				if err != nil || img.Bounds().Dx() != 496 || img.Bounds().Dy() != 1080 {
					t.Fatal("invalid screenshot", err)
				}
			}
			// Keep the image conversion contract when retiring the old radio simulator.
			jpg := filepath.Join(dir, "scaled.jpg")
			if r := run(t, a, "screenshot", "--format", "jpeg", "--max-size", "540", "--output", jpg); r.Status != "ok" || r.Data["width"] != 248.0 || r.Data["height"] != 540.0 {
				t.Fatal(r)
			}
			file, err := os.Open(jpg)
			if err != nil {
				t.Fatal(err)
			}
			img, err := jpeg.Decode(file)
			file.Close()
			if err != nil || img.Bounds().Dx() != 248 || img.Bounds().Dy() != 540 {
				t.Fatal("invalid scaled JPEG", err)
			}
			shot := mcp("screenshot", map[string]any{})
			if shot["isError"] == true {
				t.Fatal(shot)
			}
			imageFound := false
			for _, item := range shot["content"].([]any) {
				part := item.(map[string]any)
				if part["type"] == "image" {
					raw, err := base64.StdEncoding.DecodeString(part["data"].(string))
					if err != nil || !bytes.HasPrefix(raw, []byte("\x89PNG")) {
						t.Fatal("invalid MCP image")
					}
					imageFound = true
				}
			}
			if !imageFound {
				t.Fatal("missing MCP screenshot")
			}
			if r := run(t, a, "logs", "--board"); r.Status != "ok" || !strings.Contains(fmt.Sprint(r.Data["note"]), "no log") {
				t.Fatal(r)
			}
			support := filepath.Join(dir, "support.txt")
			if r := run(t, a, "logs", "--save", "--output", support); r.Status != "ok" {
				t.Fatal(r)
			}
			saved, err := os.ReadFile(support)
			if err != nil {
				t.Fatal(err)
			}
			for _, section := range []string{"== board state ==", "\"control_transport\":\"serial\"", "== board log ==", "no log", "== CLI log =="} {
				if !strings.Contains(string(saved), section) {
					t.Fatalf("support export missing %q: %s", section, saved)
				}
			}
			if r := run(t, a, "io", "text", "12345678901234567890"); r.Status != "ok" {
				t.Fatal(r)
			}
			if r := mcp("scroll", map[string]any{"amount": float64(-1)}); r["isError"] == true {
				t.Fatal(r)
			}
			if r := run(t, a, "io", "scroll", "4"); r.Status != "error" || !strings.Contains(r.Error, "test rejection") {
				t.Fatal(r)
			}
			if r := mcp("scroll", map[string]any{"amount": float64(4)}); r["isError"] != true {
				t.Fatal(r)
			}
			if r := run(t, a, "io", "tap", "247.5,539.5"); r.Status != "ok" {
				t.Fatal(r)
			}
			if r := mcp("tap", map[string]any{"x": 247.5, "y": 539.5}); r["isError"] == true {
				t.Fatal(r)
			}
			commands, opens := board.snapshot()
			if opens != 1 || strings.Count(commands, "output ") != 1 || !strings.Contains(commands, "output "+output) {
				t.Fatal(commands, opens)
			}
			if strings.Count(commands, "abs 16384 16384|click") != 2 || strings.Contains(commands, "home|") {
				t.Fatal(commands)
			}
			text := "type 12345678901234567890"
			if output == "ble" {
				text = "type 1234567890123456|type 7890"
			}
			if !strings.Contains(commands, text) {
				t.Fatal(commands)
			}
			before := commands
			for _, arg := range [][]string{{"io", "scroll", "5"}, {"net", "via", "wifi"}, {"net", "join", "unused", "unused"}} {
				if r := run(t, a, arg...); r.Status != "error" {
					t.Fatal(arg, r)
				}
			}
			after, _ := board.snapshot()
			if after != before {
				t.Fatal("rejected controls wrote to serial", before, after)
			}
			// Pair/unpair are compatibility operations, not radio pairing commands.
			if r := run(t, a, "pair"); r.Status != "ok" {
				t.Fatal(r)
			}
			if r := run(t, a, "unpair"); r.Status != "ok" {
				t.Fatal(r)
			}
			commands, _ = board.snapshot()
			if strings.Contains(commands, "pair") {
				t.Fatal(commands)
			}
		})
	}
}
