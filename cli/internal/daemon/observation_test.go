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
	"sync/atomic"
	"testing"
	"time"
)

func TestRepeatedFrameIDDoesNotRefreshCaptureTime(t *testing.T) {
	original := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at := original
		if requests.Add(1) > 1 {
			at = time.Now() // A repeated ID must not gain a new capture time on fetch.
		}
		w.Header().Set("X-Frame-Id", "unchanged")
		w.Header().Set("X-Frame-Time", fmt.Sprintf("%.3f", float64(at.UnixMilli())/1000))
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	}))
	defer server.Close()
	c := newConn("test", "test", "")
	c.frameURL, c.frameStop = server.URL, make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); c.pollFrames() }()
	defer func() { close(c.frameStop); <-done }()
	deadline := time.Now().Add(3 * time.Second)
	for requests.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if requests.Load() < 3 {
		t.Fatal("poller did not repeat request")
	}
	_, m, err := c.observationSnapshot(context.Background())
	if err != nil || m["frame_time"] != float64(original.UnixMilli())/1000 || m["state"] != "observation_only" {
		t.Fatal(m, err)
	}
}

func TestUnavailableFrameSource(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	_, _, _, err := fetchFrame(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "frame source unreachable") {
		t.Fatal(err)
	}
}

func TestStaticSnapshotKeepsTimestampAndCannotSatisfyFresh(t *testing.T) {
	c := newConn("test", "test", "")
	c.frame = image.NewRGBA(image.Rect(0, 0, 10, 10))
	c.frameID = "source:original"
	c.captured = time.Now().Add(-time.Minute)
	c.lastAction = time.Now().Add(-time.Second)
	c.lastShot = time.Now() // Already waited for this action; repeated observation must not wait again.
	original := c.captured
	for i := 0; i < 2; i++ {
		img, m, err := c.observationSnapshot(context.Background())
		if err != nil || img == nil || m["state"] != "observation_only" || m["after_action"] != false || m["mirroring_state"] != "unknown" {
			t.Fatalf("%v %v", m, err)
		}
		if m["frame_time"] != float64(original.UnixMilli())/1000 || m["frame_id"] != "source:original" || c.captured != original {
			t.Fatal("timestamp fabricated")
		}
	}
	if c.Fresh(context.Background(), c.lastAction, 20*time.Millisecond) != nil {
		t.Fatal("old observation satisfied post-action wait")
	}
	c.frameErr = errors.New("frame source unreachable")
	if img, _, err := c.observationSnapshot(context.Background()); err == nil || img != nil {
		t.Fatal("served cache after source failure")
	}
	if c.Fresh(context.Background(), original.Add(-time.Second), 20*time.Millisecond) != nil {
		t.Fatal("unavailable source cache satisfied fresh wait")
	}
}

func TestSnapshotLabelsGenuinePostActionFrame(t *testing.T) {
	c := newConn("test", "test", "")
	c.frame = image.NewRGBA(image.Rect(0, 0, 10, 10))
	c.frameID = "new"
	c.lastAction = time.Now().Add(-time.Second)
	c.captured = time.Now()
	c.lastShot = time.Now()
	_, m, err := c.observationSnapshot(context.Background())
	if err != nil || m["state"] != "after_action_frame" || m["after_action"] != true || m["effect_verified"] != false {
		t.Fatal(m, err)
	}
}

func TestDisconnectSignalAndNoFrameDifferFromOldImage(t *testing.T) {
	for _, kind := range []string{"disconnected", "empty"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "disconnected" {
					w.Header().Set("X-Mirroring-State", "disconnected")
					w.WriteHeader(200)
				} else {
					w.WriteHeader(204)
				}
			}))
			defer server.Close()
			_, _, _, err := fetchFrame(context.Background(), server.URL)
			want := "no frame"
			if kind == "disconnected" {
				want = "mirroring disconnected"
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatal(err)
			}
		})
	}
}
