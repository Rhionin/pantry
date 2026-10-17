package main

import (
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRunHTTPServerFlushesWhenStopped(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	flushed := make(chan struct{})
	srv := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(srv, stop, func() { close(flushed) })
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not listen: %v", err)
		}
	}
	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
	select {
	case <-flushed:
	default:
		t.Fatal("flush was not called")
	}
}

func TestRunHTTPServerFlushesOnListenError(t *testing.T) {
	flushed := false
	srv := &http.Server{Addr: "127.0.0.1:-1", Handler: http.NewServeMux()}
	err := runHTTPServer(srv, make(chan struct{}), func() { flushed = true })
	if err == nil {
		t.Fatal("expected a listen error")
	}
	if !flushed {
		t.Fatal("flush was not called")
	}
}
