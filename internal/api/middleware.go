package api

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// rateLimiter defines a memory-based IP/Key rate limiter.
type rateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newRateLimiter(r rate.Limit, b int) *rateLimiter {
	rl := &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}

	// Clean up stale limiters every hour to prevent memory leaks in a real system.
	// For simplicity, we just keep them here.
	return rl
}

func (rl *rateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}

	return limiter
}

// RateLimitMiddleware applies a token bucket rate limit based on the X-API-Key or IP address.
func RateLimitMiddleware(requestsPerSecond float64, burst int) func(http.Handler) http.Handler {
	rl := newRateLimiter(rate.Limit(requestsPerSecond), burst)
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identifier := r.Header.Get("X-API-Key")
			if identifier == "" {
				identifier = r.RemoteAddr
			}

			limiter := rl.getLimiter(identifier)
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, map[string]any{
					"error": map[string]string{
						"code":    "TooManyRequests",
						"message": "API rate limit exceeded. Please try again later.",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
