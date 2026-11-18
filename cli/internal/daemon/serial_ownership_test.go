package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSerialPortSelectionAndSingleOwner(t *testing.T) {
	frame := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer frame.Close()
	t.Setenv("ESP32AGENT_FRAME_URL", frame.URL)
	t.Setenv("ESP32AGENT_HID_OUTPUT", "")
	t.Setenv("ESP32AGENT_PORT", "COMenv")
	var mu sync.Mutex
	var opened []string
	s := &Server{OpenSerial: func(port string) (commandLink, error) {
		mu.Lock()
		opened = append(opened, port)
		mu.Unlock()
		return &testSerial{up: 1000}, nil
	}}
	var wg sync.WaitGroup
	connections := make(chan *Conn, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := s.serialConn(context.Background(), "")
			if e != nil {
				t.Error(e)
				return
			}
			connections <- c
		}()
	}
	wg.Wait()
	close(connections)
	var first *Conn
	for c := range connections {
		if first == nil {
			first = c
		}
		if first != c {
			t.Error("multiple owners")
		}
	}
	if first != nil {
		defer first.close()
	}
	c, e := s.serialConn(context.Background(), "COMexplicit")
	if e != nil {
		t.Fatal(e)
	}
	defer c.close()
	if strings.Join(opened, "|") != "COMenv|COMexplicit" {
		t.Fatal(opened)
	}
	if c.hidOutput != "usb" {
		t.Fatal("default output", c.hidOutput)
	}
}

func TestSerialMissingOrOccupiedPortDoesNotFallback(t *testing.T) {
	t.Setenv("ESP32AGENT_PORT", "")
	t.Setenv("ESP32AGENT_HID_OUTPUT", "usb")
	opens := 0
	s := &Server{OpenSerial: func(string) (commandLink, error) { opens++; return nil, errors.New("port occupied") }}
	if _, e := s.serialConn(context.Background(), ""); e == nil {
		t.Fatal("missing port accepted")
	}
	if opens != 0 {
		t.Fatal("port guessed")
	}
	if _, e := s.serialConn(context.Background(), "COMbusy"); e == nil || !strings.Contains(e.Error(), "port occupied") {
		t.Fatal(e)
	}
	if opens != 1 || len(s.conns) != 0 {
		t.Fatal("fallback/retry", opens, s.conns)
	}
}
