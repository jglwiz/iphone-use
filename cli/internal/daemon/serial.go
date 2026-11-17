package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/onehumanbeing/esp32-iphone-agent/cli/internal/seriallink"
)

type commandLink interface {
	Send(context.Context, string) (string, error)
	Close() error
}

func (s *Server) serialConn(ctx context.Context, selector string) (*Conn, error) {
	s.linkMu.Lock()
	defer s.linkMu.Unlock()
	port := selector
	if port == "" {
		port = os.Getenv("ESP32AGENT_PORT")
	}
	if port == "" {
		return nil, errors.New("set ESP32AGENT_PORT or --device to a verified serial port (devices lists ports without probing them)")
	}
	s.mu.Lock()
	if s.conns == nil {
		s.conns = map[string]*Conn{}
	}
	c := s.conns[port]
	s.mu.Unlock()
	if c != nil {
		return c, nil
	}
	open := s.OpenSerial
	if open == nil {
		open = func(p string) (commandLink, error) { return seriallink.Open(p) }
	}
	l, err := open(port)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", port, err)
	}
	c = newConn(port, "ESP32 serial "+port, "")
	c.serial = l
	c.via = "serial"
	c.links = 1
	c.actionGate = make(chan struct{}, 1)
	c.actionGate <- struct{}{}
	c.frameURL = os.Getenv("ESP32AGENT_FRAME_URL")
	if c.frameURL == "" {
		c.frameURL = "http://127.0.0.1:8892/frame"
	}
	if err = c.refreshSerial(ctx); err != nil {
		l.Close()
		return nil, err
	}
	c.frameStop = make(chan struct{})
	go c.pollFrames()
	s.mu.Lock()
	s.conns[port] = c
	s.mu.Unlock()
	return c, nil
}

func (c *Conn) refreshSerial(ctx context.Context) error {
	raw, err := c.serial.Send(ctx, "status")
	if err != nil {
		return err
	}
	var st map[string]any
	if json.Unmarshal([]byte(raw), &st) != nil || st["fw"] == nil || st["usb"] == nil {
		return errors.New("port did not return Handsy board status")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.status["up"].(float64); ok && num(st["up"]) < old {
		c.serialFault = errors.New("board rebooted; input outcome unknown; restart daemon after inspection")
	}
	st["control_transport"] = "serial"
	st["hid_output"] = "usb"
	st["port"] = c.ID
	st["screen_source"] = "http"
	c.status = st
	c.statusAt = time.Now()
	return c.serialFault
}

func (s *Server) doSerial(ctx context.Context, req Request) Response {
	switch req.Cmd {
	case "devices":
		ports, err := seriallink.Ports()
		if err != nil {
			return fail(err)
		}
		list := []deviceInfo{}
		for _, p := range ports {
			d:=deviceInfo{ID:p,Name:p,Platform:"ios",Type:"serial-port",State:"unverified"}
   s.mu.Lock(); if c:=s.conns[p];c!=nil { d.Name=c.Name;d.Type="esp32agent";d.State="online";d.Connected=true };s.mu.Unlock()
   list=append(list,d)
		}
		return ok(map[string]any{"devices": list})
	case "unpair":
		return ok(map[string]string{"message": "serial needs no Bluetooth pairing; no pairing data changed"})
	}
	c, err := s.serialConn(ctx, req.Device)
	if err != nil {
		return fail(err)
	}
	// The entire operation (including release and post-action capture) has one owner.
	select {
	case <-ctx.Done():
		return fail(ctx.Err())
	case <-c.actionGate:
	}
	defer func() { c.actionGate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	switch req.Cmd {
	case "info", "pair":
		if err := c.refreshSerial(ctx); err != nil {
			return fail(err)
		}
		st, age := c.Status()
		st["id"], st["name"], st["age_ms"] = c.ID, c.Name, age.Milliseconds()
		if req.Cmd == "pair" {
			st["message"] = "serial connected; Bluetooth pairing is not required"
		}
		return ok(st)
	case "screenshot":
		return s.screenshot(ctx, c, req)
	case "calibrate":
		if err := c.refreshSerial(ctx); err != nil {
			return fail(err)
		}
		if num(c.status["abs"]) != 1 {
			return fail(errors.New("firmware lacks native absolute pointer"))
		}
		return ok(map[string]any{"mode": "native_absolute", "message": "no relative calibration used; inspect targets on the current phone", "effect_verified": false})
	case "tap", "longpress", "swipe", "text", "keys", "button", "scroll":
		return s.serialIO(ctx, c, req)
	case "send":
		// Keep the diagnostic API but never expose the old relative controller, pairing or video transports.
		fields := strings.Fields(req.Text)
		if len(fields) == 0 {
			return fail(errors.New("empty command"))
		}
		switch fields[0] {
		case "status", "ping", "abs", "click", "press_for", "release", "reset", "scroll_by", "scroll_burst", "key", "type", "homescreen", "hidtrace", "logs", "crash":
		default:
			return fail(errors.New("command not available through the serial control API"))
		}
		if err := c.refreshSerial(ctx); err != nil {
			return fail(err)
		}
		st, _ := c.Status()
		if owner, _ := st["owner"].(string); owner != "" && owner != "none" {
			return fail(errors.New("another controller owns the board"))
		}
		reply, err := c.Send(ctx, req.Text)
		if fields[0] != "status" && fields[0] != "ping" && fields[0] != "logs" && fields[0] != "hidtrace" && fields[0] != "crash" {
			c.mu.Lock()
			c.lastAction = time.Now()
			c.mu.Unlock()
		}
		if err != nil {
			return fail(err)
		}
		return ok(map[string]string{"reply": reply})
	case "boardlog":
		return s.boardLog(ctx, c)
	case "net":
		if req.Text != "" {
			return fail(errors.New("network transport configuration removed; PC control uses serial"))
		}
		return ok(map[string]any{"via": "serial", "prefer": "serial", "message": "PC Bluetooth/Wi-Fi control removed; screen source is separate"})
	case "listen":
		return fail(errors.New("phone audio is not supplied by the serial/HTTP screenshot path"))
	default:
		return fail(fmt.Errorf("unknown command %q", req.Cmd))
	}
}
