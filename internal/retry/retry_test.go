package retry

import (
	"errors"
	"testing"
	"time"
)

func TestDoSucceedsImmediately(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		return nil
	}, WithInitialBackoff(time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDoRetriesOnError(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	}, WithMaxAttempts(3), WithInitialBackoff(time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDoExhaustsAttempts(t *testing.T) {
	calls := 0
	sentinel := errors.New("persistent")
	err := Do(func() error {
		calls++
		return sentinel
	}, WithMaxAttempts(4), WithInitialBackoff(time.Millisecond))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected 4 calls, got %d", calls)
	}
}

func TestDoRespectsBackoff(t *testing.T) {
	start := time.Now()
	calls := 0
	_ = Do(func() error {
		calls++
		if calls < 3 {
			return errors.New("fail")
		}
		return nil
	}, WithMaxAttempts(3), WithInitialBackoff(10*time.Millisecond), WithMultiplier(1.0))
	elapsed := time.Since(start)
	// With 2 sleeps of ~10ms (jittered to 5-10ms each), expect at least 8ms total.
	if elapsed < 8*time.Millisecond {
		t.Fatalf("expected backoff delay, but elapsed was only %v", elapsed)
	}
}

func TestDoSucceedsAfterOneFailure(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		if calls == 1 {
			return errors.New("first fail")
		}
		return nil
	}, WithMaxAttempts(2), WithInitialBackoff(time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestDoSingleAttempt(t *testing.T) {
	sentinel := errors.New("fail")
	err := Do(func() error {
		return sentinel
	}, WithMaxAttempts(1))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}
