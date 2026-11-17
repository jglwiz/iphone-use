package seriallink

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func wire(t *testing.T, respond func(string) string) (*Link, *atomic.Int32) {
	t.Helper()
	pc, board := net.Pipe()
	l := New(pc)
	count := &atomic.Int32{}
	t.Cleanup(func() { l.Close(); board.Close() })
	go func() {
		r := bufio.NewScanner(board)
		for r.Scan() {
			count.Add(1)
			reply := respond(r.Text())
			for _, ch := range []byte(reply) {
				if _, err := board.Write([]byte{ch}); err != nil {
					return
				}
			}
		}
	}()
	return l, count
}

func TestSplitLogsAndMatchingID(t *testing.T) {
	l, _ := wire(t, func(cmd string) string {
		id, _, _ := strings.Cut(cmd, " ")
		return "boot log\r\n7 ok\r\n" + id + " -> ignored debug\n" + id + " ok done\r\n"
	})
	got, err := l.Send(context.Background(), "click")
	if err != nil || got != "done" {
		t.Fatalf("%q %v", got, err)
	}
}
func TestStatusAndExplicitError(t *testing.T) {
	l, _ := wire(t, func(cmd string) string {
		id, rest, _ := strings.Cut(cmd, " ")
		if rest == "status" {
			return "0 {\"usb\":\"ok\"}\n"
		}
		return id + " err invalid amount\n"
	})
	if got, err := l.Send(context.Background(), "status"); err != nil || got != `{"usb":"ok"}` {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := l.Send(context.Background(), "scroll_by 5"); err == nil || err.Error() != "invalid amount" {
		t.Fatal(err)
	}
	// A definitive error does not poison the link.
	if _, err := l.Send(context.Background(), "status"); err != nil {
		t.Fatal(err)
	}
}
func TestTimeoutLatchesAndDoesNotReplay(t *testing.T) {
	l, n := wire(t, func(string) string { return "" })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := l.Send(ctx, "click"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatal(err)
	}
	if _, err := l.Send(context.Background(), "click"); err == nil {
		t.Fatal("replayed")
	}
	if n.Load() != 1 {
		t.Fatal(n.Load())
	}
}
func TestRejectInjectionBeforeWrite(t *testing.T) {
	l, n := wire(t, func(string) string { return "" })
	for _, cmd := range []string{"", "click\n2 homescreen", "key a\r", "type \x00", strings.Repeat("a", 451)} {
		if _, err := l.Send(context.Background(), cmd); err == nil {
			t.Fatal(cmd)
		}
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}

type shortPort struct{ io.ReadCloser }

func (shortPort) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestShortWritePoisonsSession(t *testing.T) {
	r, w := io.Pipe()
	l := New(shortPort{r})
	defer l.Close()
	defer w.Close()
	if _, err := l.Send(context.Background(), "click"); err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatal(err)
	}
}
func TestCancelledBeforeSend(t *testing.T) {
	l, n := wire(t, func(string) string { return "" })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Send(ctx, "click"); err == nil {
		t.Fatal("accepted")
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}

func TestDisconnectAfterWriteIsUnknown(t *testing.T) {
 pc, board := net.Pipe()
 l := New(pc)
 defer l.Close()
 go func() { bufio.NewScanner(board).Scan(); board.Close() }()
 if _, err := l.Send(context.Background(), "click"); err == nil || !strings.Contains(err.Error(), "unknown") {
  t.Fatal(err)
 }
 if _, err := l.Send(context.Background(), "click"); err == nil { t.Fatal("replayed after disconnect") }
}
