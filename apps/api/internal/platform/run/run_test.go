package run

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeGracefulShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.WriteHeader(200) })}
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, server, listener, time.Second, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}()
	clientDone := make(chan error, 1)
	go func() {
		client := http.Client{Timeout: 2 * time.Second}
		r, e := client.Get("http://" + listener.Addr().String())
		if e == nil {
			r.Body.Close()
			if r.StatusCode != 200 {
				e = context.Canceled
			}
		}
		clientDone <- e
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("server did not start")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before draining request")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown stuck")
	}
	if err = <-clientDone; err != nil {
		t.Fatal(err)
	}
}
func TestWorkerStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Worker(ctx, slog.New(slog.NewJSONHandler(io.Discard, nil))); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}
