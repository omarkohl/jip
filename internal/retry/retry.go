package retry

import (
	"math"
	"math/rand/v2"
	"time"
)

// config holds retry parameters.
type config struct {
	maxAttempts    int
	initialBackoff time.Duration
	multiplier     float64
	maxBackoff     time.Duration
}

// Option configures retry behavior.
type Option func(*config)

// WithMaxAttempts sets the maximum number of attempts (default 3).
func WithMaxAttempts(n int) Option {
	return func(c *config) { c.maxAttempts = n }
}

// WithInitialBackoff sets the initial backoff duration (default 1s).
func WithInitialBackoff(d time.Duration) Option {
	return func(c *config) { c.initialBackoff = d }
}

// WithMultiplier sets the backoff multiplier (default 2.0).
func WithMultiplier(m float64) Option {
	return func(c *config) { c.multiplier = m }
}

// WithMaxBackoff sets the maximum backoff duration (default 30s).
func WithMaxBackoff(d time.Duration) Option {
	return func(c *config) { c.maxBackoff = d }
}

// Do calls fn up to maxAttempts times, sleeping with exponential backoff
// and jitter between attempts. Returns the last error if all attempts fail.
func Do(fn func() error, opts ...Option) error {
	cfg := config{
		maxAttempts:    3,
		initialBackoff: 1 * time.Second,
		multiplier:     2.0,
		maxBackoff:     30 * time.Second,
	}
	for _, o := range opts {
		o(&cfg)
	}

	var err error
	for attempt := range cfg.maxAttempts {
		err = fn()
		if err == nil {
			return nil
		}
		if attempt < cfg.maxAttempts-1 {
			backoff := float64(cfg.initialBackoff) * math.Pow(cfg.multiplier, float64(attempt))
			if backoff > float64(cfg.maxBackoff) {
				backoff = float64(cfg.maxBackoff)
			}
			// Add jitter: 50-100% of the computed backoff.
			jittered := time.Duration(backoff * (0.5 + rand.Float64()*0.5))
			time.Sleep(jittered)
		}
	}
	return err
}
