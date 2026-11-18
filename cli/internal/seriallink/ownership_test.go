package seriallink

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCancelledAfterWriteLatchesWithoutLateReplay(t *testing.T) {
	pc, board := net.Pipe()
	link := New(pc)
	defer link.Close()
	defer board.Close()
	received := make(chan string, 1)
	release := make(chan struct{})
	written := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(board)
		if scanner.Scan() {
			id, _, _ := strings.Cut(scanner.Text(), " ")
			received <- scanner.Text()
			<-release
			board.Write([]byte(id + " ok\n"))
		}
		close(written)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := link.Send(ctx, "click"); result <- err }()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("no serial write")
	}
	cancel()
	if err := <-result; err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatal(err)
	}
	close(release)
	<-written
	for _, cmd := range []string{"click", "status", "output usb"} {
		if _, err := link.Send(context.Background(), cmd); err == nil || !strings.Contains(err.Error(), "not replayed") {
			t.Fatal(cmd, err)
		}
	}
	// No later command reached the device, even though the old success arrived.
	board.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if n, _ := board.Read(make([]byte, 64)); n != 0 {
		t.Fatal("unexpected replay")
	}
}

func TestQueuedCancelledCommandNeverWrites(t *testing.T) {
	pc, board := net.Pipe()
	link := New(pc)
	defer link.Close()
	defer board.Close()
	first := make(chan string, 1)
	release := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(board)
		if scanner.Scan() {
			id, _, _ := strings.Cut(scanner.Text(), " ")
			first <- id
			<-release
			board.Write([]byte(id + " ok\n"))
		}
	}()
	done := make(chan error, 1)
	go func() { _, err := link.Send(context.Background(), "click"); done <- err }()
	<-first
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := link.Send(ctx, "homescreen"); err == nil {
		t.Fatal("queued command accepted")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	board.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if n, _ := board.Read(make([]byte, 64)); n != 0 {
		t.Fatal("cancelled queued input sent")
	}
}
