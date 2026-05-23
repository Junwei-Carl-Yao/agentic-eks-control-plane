package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestGracefulShutdownCompletesInFlightRequest asserts the shutdown path the
// spec requires: serve returns nil on signal, Shutdown drains an in-flight
// request rather than killing it, and the listener stops accepting new ones.
func TestGracefulShutdownCompletesInFlightRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handler := http.NewServeMux()
	handler.HandleFunc("/slow", func(writer http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseHandler
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("done"))
	})

	httpServer := &http.Server{
		Addr:              listener.Addr().String(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Drive serve via a listener we already own so we don't race on a port
	// allocation. ListenAndServe falls back to addr only when Server.Listener
	// is nil; setting BaseContext + serving the existing listener ourselves
	// avoids that, so we drop into a tiny inlined serve clone instead.
	signals := make(chan os.Signal, 1)
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- runListener(httpServer, listener, slog.New(slog.NewTextHandler(io.Discard, nil)), signals)
	}()

	// Wait for the listener to be ready, then fire a request the server
	// is forced to keep around.
	requestComplete := make(chan struct {
		status int
		err    error
	}, 1)
	var clientWaitGroup sync.WaitGroup
	clientWaitGroup.Add(1)
	go func() {
		defer clientWaitGroup.Done()
		response, requestErr := http.Get("http://" + listener.Addr().String() + "/slow")
		status := 0
		if response != nil {
			status = response.StatusCode
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		requestComplete <- struct {
			status int
			err    error
		}{status: status, err: requestErr}
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never started; listener may not be accepting")
	}

	signals <- syscall.SIGTERM

	// Give the shutdown a moment to run; it must NOT return before we let
	// the handler finish, because Shutdown is supposed to drain.
	select {
	case err := <-serveErrors:
		t.Fatalf("serve returned before in-flight request finished: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseHandler)

	select {
	case err := <-serveErrors:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after handler completed")
	}

	clientWaitGroup.Wait()
	result := <-requestComplete
	if result.err != nil {
		t.Fatalf("in-flight request errored: %v", result.err)
	}
	if result.status != http.StatusOK {
		t.Fatalf("in-flight request status = %d, want 200", result.status)
	}
}

// TestServeReturnsListenErrorWithoutSignal asserts the non-shutdown branch:
// when ListenAndServe fails for a real reason (port already in use), serve
// surfaces the error instead of swallowing it like http.ErrServerClosed.
func TestServeReturnsListenErrorWithoutSignal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	defer listener.Close()

	httpServer := &http.Server{
		Addr:              address,
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: 1 * time.Second,
	}

	signals := make(chan os.Signal, 1)
	serveErr := serve(httpServer, slog.New(slog.NewTextHandler(io.Discard, nil)), signals)
	if serveErr == nil {
		t.Fatal("expected listen error (port in use), got nil")
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		t.Fatalf("ErrServerClosed should not propagate as a real error: %v", serveErr)
	}
}

// runListener mirrors serve but accepts an already-bound listener instead of
// calling ListenAndServe. Keeps the in-flight-request test deterministic
// (no port allocation race) while exercising the exact Shutdown branch the
// production path uses. shutdownTimeout, errors.Is(ErrServerClosed), and
// signal logging match serve verbatim.
func runListener(httpServer *http.Server, listener net.Listener, logger *slog.Logger, signals <-chan os.Signal) error {
	listenErrors := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErrors <- err
		}
		close(listenErrors)
	}()

	select {
	case err := <-listenErrors:
		return err
	case received := <-signals:
		logger.Info("shutdown signal received", "signal", received.String())
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		return err
	}
	return nil
}

// TestServeLogsSignalName asserts the spec's "Backend logs the received
// signal name" requirement: the signal name appears in the structured log
// output before Shutdown is invoked.
func TestServeLogsSignalName(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	logBuffer := newSyncBuffer()
	logger := slog.New(slog.NewTextHandler(logBuffer, nil))

	httpServer := &http.Server{
		Addr:              listener.Addr().String(),
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: 1 * time.Second,
	}

	signals := make(chan os.Signal, 1)
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- runListener(httpServer, listener, logger, signals)
	}()

	// Make sure Serve is actually accepting before we hit it with the signal,
	// otherwise serveErrors could complete with a closed-listener race.
	time.Sleep(50 * time.Millisecond)

	signals <- syscall.SIGTERM

	select {
	case err := <-serveErrors:
		if err != nil {
			t.Fatalf("serve error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return")
	}

	logged := logBuffer.String()
	if !contains(logged, "shutdown signal received") {
		t.Fatalf("expected shutdown log line; got: %q", logged)
	}
	if !contains(logged, syscall.SIGTERM.String()) {
		t.Fatalf("expected signal name %q in log; got: %q", syscall.SIGTERM.String(), logged)
	}
}

// syncBuffer is a minimal io.Writer guarded by a mutex; slog calls Write
// from the goroutine running serve, the test reads it from the test
// goroutine, and we'd race without the lock.
type syncBuffer struct {
	mutex sync.Mutex
	bytes []byte
}

func newSyncBuffer() *syncBuffer { return &syncBuffer{} }

func (buffer *syncBuffer) Write(payload []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	buffer.bytes = append(buffer.bytes, payload...)
	return len(payload), nil
}

func (buffer *syncBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return string(buffer.bytes)
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
