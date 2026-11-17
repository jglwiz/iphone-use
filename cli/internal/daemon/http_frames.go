package daemon

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
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
		return nil, time.Time{}, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, time.Time{}, "", fmt.Errorf("frame HTTP %d", resp.StatusCode)
	}
	ts, err := strconv.ParseFloat(resp.Header.Get("X-Frame-Time"), 64)
	id := resp.Header.Get("X-Frame-Id")
	if err != nil || id == "" || ts <= 0 {
		return nil, time.Time{}, "", errors.New("frame lacks capture timestamp or ID")
	}
	at := time.UnixMilli(int64(ts * 1000))
	if time.Since(at) > 5*time.Second || time.Until(at) > time.Second {
		return nil, at, id, errors.New("frame stale or clock invalid")
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

func (c *Conn) httpFrame(ctx context.Context) (image.Image, error) {
	c.mu.Lock()
	since := c.lastAction
	c.mu.Unlock()
	// Wait for a genuinely new frame after an action, allowing for the phone's display delay.
	if !since.IsZero() {
		since = since.Add(300 * time.Millisecond)
	}
	img := c.Fresh(ctx, since, 4*time.Second)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frameErr != nil {
		return nil, c.frameErr
	}
	if img == nil || time.Since(c.captured) > 5*time.Second {
		return nil, errors.New("no fresh captured frame after the action; input was not replayed")
	}
	c.lastShot = time.Now()
	return img, nil
}
