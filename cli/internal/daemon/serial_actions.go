package daemon

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/onehumanbeing/esp32-iphone-agent/cli/internal/pointer"
)

func (s *Server) serialIO(ctx context.Context, c *Conn, req Request) Response {
	ctx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	err := func() (err error) {
		if err = c.refreshSerial(ctx); err != nil {
			return err
		}
		st, _ := c.Status()
		if owner, _ := st["owner"].(string); owner != "" && owner != "none" {
			return errors.New("another controller owns the board; serial input blocked")
		}
		if err := serialReady(st); err != nil {
			return err
		}
		send := func(cmd string) error { _, err := c.Send(ctx, cmd); return err }
		switch req.Cmd {
		case "scroll":
			amount, e := strconv.Atoi(req.Text)
			if e != nil || amount < -4 || amount > 4 {
				return errors.New("scroll amount must be an integer -4..4 (positive down)")
			}
			return send(fmt.Sprintf("scroll_by %d", amount))
		case "text":
			if strings.ContainsAny(req.Text, "\r\n\x00") {
				return errors.New("text cannot contain line breaks or NUL; use explicit keys")
			}
			text := pointer.Typeable(req.Text)
			if text == "" {
				return errors.New("nothing typeable in text")
			}
			chunk := textChunk
			if st["hid_output"] == "ble" {
				chunk = 16
			}
			for len(text) > 0 {
				n := min(len(text), chunk)
				if _, e := c.pointer(s.Dir).Type(ctx, text[:n]); e != nil {
					return e
				}
				text = text[n:]
			}
			return nil
		case "keys":
			return device{c}.Send(ctx, "key "+strings.ToLower(req.Text))
		case "button":
			if !strings.EqualFold(req.Text, "HOME") {
				return errors.New("only HOME is supported")
			}
			return send("homescreen") // One explicit action; never silently add Cmd+H and a swipe.
		}
		if num(st["abs"]) != 1 {
			return errors.New("native absolute HID firmware required; no relative fallback")
		}
		n := 2
		if req.Cmd == "swipe" {
			n = 4
		}
		if len(req.Pts) != n {
			return errors.New("wrong number of coordinates")
		}
		if req.Count < 0 || req.Count > 10 {
			return errors.New("tap count must be 1..10")
		}
		if req.Ms < 0 || req.Ms > 60000 {
			return errors.New("duration must be 1..60000 ms")
		}
		img, e := c.httpFrame(ctx)
		if e != nil {
			return e
		}
		w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
		if w < 2 || h < 2 {
			return errors.New("invalid frame dimensions")
		}
		for i, v := range req.Pts {
			limit := w
			if i%2 == 1 {
				limit = h
			}
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v >= limit {
				return errors.New("coordinates outside the current screenshot")
			}
		}
		point := func(i int) error {
			return send(fmt.Sprintf("abs %d %d", int(math.Round(req.Pts[i]/(w-1)*32767)), int(math.Round(req.Pts[i+1]/(h-1)*32767))))
		}
		if err = point(0); err != nil {
			return err
		}
		if req.Aim {
			if err = c.verifyAbsolute(ctx, req.Pts[0]/(w-1), req.Pts[1]/(h-1)); err != nil {
				return err
			}
		}
		switch req.Cmd {
		case "tap":
			for i := 0; i < max(1, req.Count); i++ {
				if err = send("click"); err != nil {
					return err
				}
			}
			return nil
		case "longpress", "swipe":
			// Finite firmware hold also releases if the PC disappears. Cleanup is independent of cancellation.
			defer func() {
				releaseCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
				defer done()
				_, rerr := c.Send(releaseCtx, "reset")
				err = errors.Join(err, rerr)
			}()
			ms := req.Ms
			if ms == 0 {
				ms = 1000
			}
			if req.Cmd == "swipe" {
				ms = 5000
			}
			if err = send(fmt.Sprintf("press_for %d", ms)); err != nil {
				return err
			}
			if req.Cmd == "longpress" {
				return sleepCtx(ctx, time.Duration(ms)*time.Millisecond)
			}
			if err = point(2); err != nil {
				return err
			}
			return sleepCtx(ctx, 250*time.Millisecond)
		}
		return errors.New("unsupported serial action")
	}()
	c.mu.Lock()
	c.lastAction = time.Now()
	c.mu.Unlock()
	s.recordAction(c, req, err)
	if err != nil {
		return fail(err)
	}
	st, _ := c.Status()
	out := map[string]any{"message": req.Cmd + " done", "effect_verified": false, "transport": "serial", "hid_output": st["hid_output"]}
	if req.Cmd == "swipe" {
		out["note"] = "pointer drag submitted; page scrolling is not guaranteed; use scroll for content"
	}
	if req.Cmd == "text" && pointer.HasHan(req.Text) {
		out["typed_as"] = pointer.Typeable(req.Text)
		out["note"] = "requires the iPhone pinyin keyboard"
	}
	return ok(out)
}

// Explicit --aim observes an absolute out-and-back movement. It never nudges relatively
// or learns a correction from a failed response. A miss blocks the following click.
func (c *Conn) verifyAbsolute(ctx context.Context, x, y float64) error {
	observe := func() image.Image {
		at := time.Now().Add(300 * time.Millisecond)
		return c.Fresh(ctx, at, 3*time.Second)
	}
	before := observe()
	shift := 0.02
	if x > 0.8 {
		shift = -shift
	}
	move := func(px float64) error {
		_, err := c.Send(ctx, fmt.Sprintf("abs %d %d", int(math.Round(px*32767)), int(math.Round(y*32767))))
		return err
	}
	if err := move(x + shift); err != nil {
		return err
	}
	after := observe()
	if err := move(x); err != nil {
		return err
	}
	back := observe()
	pt, ok := pointer.LocateBack(before, after, back, pointer.Point{X: x, Y: y}, shift, pointer.Point{X: .12, Y: .08})
	if !ok || math.Abs(pt.X-x) > .015 || math.Abs(pt.Y-y) > .015 {
		return errors.New("absolute pointer not visually confirmed; click blocked (no relative fallback)")
	}
	return nil
}
