// Package seriallink owns the board's existing newline command protocol.
// It never retries an input, including after an ambiguous timeout or short write.
package seriallink

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

type Port interface{ io.ReadWriteCloser }
type Link struct {
	port  Port
	gate  chan struct{}
	lines chan string
	done  chan struct{}
	once  sync.Once
	next  uint64
	fault error // protected by gate
}

func Ports() ([]string, error) { return serial.GetPortsList() }

func Open(name string) (*Link, error) {
	p, err := serial.Open(name, &serial.Mode{BaudRate: 115200, DataBits: 8, StopBits: serial.OneStopBit,
		InitialStatusBits: &serial.ModemOutputBits{DTR: false, RTS: false}})
	if err != nil {
		return nil, err
	}
	if err := p.ResetInputBuffer(); err != nil {
		p.Close()
		return nil, err
	}
	return New(p), nil
}

func New(p Port) *Link {
	l := &Link{port: p, gate: make(chan struct{}, 1), lines: make(chan string, 64), done: make(chan struct{}), next: uint64(time.Now().UnixMilli())}
	l.gate <- struct{}{}
	go func() {
		defer close(l.lines)
		r := bufio.NewScanner(p)
		r.Buffer(make([]byte, 1024), 16384)
		for r.Scan() {
			line := strings.TrimSuffix(r.Text(), "\r")
			// Boot/debug output is not a reply. Keep only numeric-id protocol lines.
			id, _, ok := strings.Cut(line, " ")
			if _, err := strconv.ParseUint(id, 10, 64); !ok || err != nil {
				continue
			}
			select {
			case l.lines <- line:
			case <-l.done:
				return
			}
		}
	}()
	return l
}

func (l *Link) Close() error {
	var err error
	l.once.Do(func() { close(l.done); err = l.port.Close() })
	return err
}

func (l *Link) Send(ctx context.Context, command string) (string, error) {
	if command == "" || strings.ContainsAny(command, "\r\n\x00") || len(command) > 450 {
		return "", errors.New("invalid serial command")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-l.gate:
	}
	defer func() { l.gate <- struct{}{} }()
	if l.fault != nil && command != "reset" {
		return "", l.fault
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	l.next++
	id := strconv.FormatUint(l.next, 10)
	if command == "status" {
		id = "0"
	} // Existing, already tested firmware's read-only status command.
	wire := []byte(id + " " + command + "\n")
	written := make(chan error, 1)
	go func() {
		n, err := l.port.Write(wire)
		if err == nil && n != len(wire) {
			err = io.ErrShortWrite
		}
		written <- err
	}()
	uncertain := func(err error) (string, error) {
		l.fault = fmt.Errorf("serial outcome unknown; not replayed; restart daemon after inspection: %w", err)
		return "", l.fault
	}
	select {
	case err := <-written:
		if err != nil {
			return uncertain(err)
		}
	case <-ctx.Done():
		l.Close()
		return uncertain(ctx.Err())
	}
	for {
		select {
		case <-ctx.Done():
			return uncertain(ctx.Err())
		case line, ok := <-l.lines:
			if !ok {
				return uncertain(io.EOF)
			}
			rid, body, _ := strings.Cut(line, " ")
			if rid != id {
				continue
			}
			if command == "status" && strings.HasPrefix(body, "{") {
				if !json.Valid([]byte(body)) {
					return uncertain(errors.New("invalid status JSON"))
				}
				return body, nil
			}
			if body == "ok" {
				return "", nil
			}
			if strings.HasPrefix(body, "ok ") {
				return strings.TrimPrefix(body, "ok "), nil
			}
			if strings.HasPrefix(body, "err ") {
				return "", errors.New(strings.TrimPrefix(body, "err "))
			}
			// Other numeric debug lines cannot complete this request.
		}
	}
}
