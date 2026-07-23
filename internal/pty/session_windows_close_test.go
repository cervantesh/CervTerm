//go:build windows

package pty

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLocalSessionCloseIsConcurrentAndIdempotent(t *testing.T) {
	sentinel := errors.New("close result")
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	session := &localSession{closePTY: func() error {
		calls.Add(1)
		close(entered)
		<-release
		return sentinel
	}}

	const closers = 32
	results := make(chan error, closers)
	var group sync.WaitGroup
	group.Add(closers)
	for index := 0; index < closers; index++ {
		go func() {
			defer group.Done()
			results <- session.Close()
		}()
	}
	<-entered
	close(release)
	group.Wait()
	close(results)

	if got := calls.Load(); got != 1 {
		t.Fatalf("native close calls = %d, want 1", got)
	}
	for err := range results {
		if !errors.Is(err, sentinel) {
			t.Fatalf("Close() error = %v, want %v", err, sentinel)
		}
	}
}
