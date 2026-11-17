package daemon

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

// Frame timestamps come from capture, never from the time we fetched an old JPEG.
func fetchFrame(ctx context.Context, url string) (image.Image, time.Time, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, time.Time{}, "", err
	}
	resp, err := frameClient.Do(req)
	if err != nil {
		return nil, time.Time{}, "", fmt.Errorf("frame source unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("X-Mirroring-State") == "disconnected" {
		return nil, time.Time{}, "", errors.New("mirroring disconnected (reported by source)")
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, time.Time{}, "", errors.New("frame source reachable but no frame; mirroring state unknown")
	}
	if resp.StatusCode != 200 {
		return nil, time.Time{}, "", fmt.Errorf("frame HTTP %d", resp.StatusCode)
	}
	ts, err := strconv.ParseFloat(resp.Header.Get("X-Frame-Time"), 64)
	id := resp.Header.Get("X-Frame-Id")
	if err != nil || id == "" || ts <= 0 || math.IsNaN(ts) || math.IsInf(ts, 0) {
		return nil, time.Time{}, "", errors.New("frame lacks capture timestamp or ID")
	}
	at := time.UnixMilli(int64(ts * 1000))
	if time.Until(at) > time.Second {
		return nil, at, id, errors.New("frame clock invalid")
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 16<<20))
	return img, at, id, err
}

var frameClient = &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 2 * time.Second}

func (c *Conn) pollFrames() {
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		img, at, id, err := fetchFrame(context.Background(), c.frameURL)
		c.mu.Lock()
		c.frameErr = err
		if err == nil && id != c.frameID {
			hash := hashImage(img)
			if c.frame == nil || hash != c.frameHash {
				c.changed = time.Now()
			}
			c.frame, c.frameHash, c.captured, c.updated, c.frameID = img, hash, at, time.Now(), id
			for ch := range c.frameSubs {
				select {
				case ch <- img:
				default:
				}
			}
			close(c.wake)
			c.wake = make(chan struct{})
		}
		c.mu.Unlock()
		select {
		case <-c.frameStop:
			return
		case <-ticker.C:
		}
	}
}

// observationSnapshot returns a usable observation, never a fabricated post-action frame.
// HTTP availability is not evidence of the phone's mirroring connection.
func (c *Conn) observationSnapshot(ctx context.Context) (image.Image, map[string]any, error) {
	c.mu.Lock()
	since, shot := c.lastAction, c.lastShot
	empty := c.frame == nil
	c.mu.Unlock()
	if empty || (!since.IsZero() && shot.Before(since)) {
		threshold := since
		if !threshold.IsZero() {
			threshold = threshold.Add(300 * time.Millisecond)
		}
		c.Fresh(ctx, threshold, 4*time.Second)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frameErr != nil {
		return nil, nil, c.frameErr
	}
	if c.frame == nil {
		return nil, nil, errors.New("frame source has no image")
	}
	age := time.Since(c.captured).Seconds()
	after := !c.lastAction.IsZero() && c.captured.After(c.lastAction.Add(300*time.Millisecond))
	freshness := "fresh"
	if age > 5 {
		freshness = "unchanged_or_stale"
	}
	state := "observation_only"
	if after && age <= 5 {
		state = "after_action_frame"
	}
	metadata := map[string]any{"state": state, "frame_time": float64(c.captured.UnixMilli()) / 1000,
		"frame_id": c.frameID, "age_seconds": age, "freshness": freshness, "after_action": after && age <= 5,
		"effect_verified": false, "source_state": "available", "mirroring_state": "unknown"}
	if c.lastAction.Equal(since) {
		c.lastShot = time.Now()
	}
	return c.frame, metadata, nil
}

func (c *Conn) httpFrame(ctx context.Context) (image.Image, error) {
	img, _, err := c.observationSnapshot(ctx)
	return img, err
}
