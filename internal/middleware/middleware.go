package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"ignite/internal/errors"

	"golang.org/x/time/rate"
)

// RateLimitMiddleware implements rate limiting to prevent DOS attacks
func RateLimitMiddleware(limiter *rate.Limiter, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				logger.Warn("Rate limit exceeded",
					slog.String("ip", r.RemoteAddr),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware adds security headers to responses
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS protection
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Force HTTPS (if applicable)
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Content Security Policy - Allow required CDN resources
		cspPolicy := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://code.jquery.com https://unpkg.com https://cdnjs.cloudflare.com; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com; " +
			"font-src 'self' https://fonts.gstatic.com; " +
			"img-src 'self' data:; " +
			"connect-src 'self'"
		w.Header().Set("Content-Security-Policy", cspPolicy)

		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	})
}

// MaxSizeMiddleware limits request body size to prevent memory exhaustion
func MaxSizeMiddleware(maxSize int64, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)
			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutMiddleware wraps handlers with a timeout
func TimeoutMiddleware(timeout time.Duration, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, "Request timeout")
	}
}

// LoggingMiddleware logs HTTP requests with structured logging
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response wrapper to capture status code
			wrapper := &responseWrapper{ResponseWriter: w, statusCode: http.StatusOK}

			defer func() {
				logger.Info("HTTP Request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("remote_addr", r.RemoteAddr),
					slog.Int("status_code", wrapper.statusCode),
					slog.Duration("duration", time.Since(start)),
					slog.String("user_agent", r.UserAgent()),
				)
			}()

			next.ServeHTTP(wrapper, r)
		})
	}
}

// responseWrapper wraps http.ResponseWriter to capture status code
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ErrorHandlerMiddleware recovers from panics and handles errors consistently
func ErrorHandlerMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("Panic recovered",
						slog.Any("error", err),
						slog.String("path", r.URL.Path),
						slog.String("method", r.Method),
					)

					if httpErr, ok := err.(error); ok {
						errors.HandleHTTPError(w, logger, httpErr)
					} else {
						http.Error(w, "Internal server error", http.StatusInternalServerError)
					}
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware handles CORS headers (if needed for API access)
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if origin == allowedOrigin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// DefaultMiddleware returns a chain of default middleware
func DefaultMiddleware(logger *slog.Logger) []func(http.Handler) http.Handler {
	// 10 requests per second with burst of 20
	limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 20)

	return []func(http.Handler) http.Handler{
		LoggingMiddleware(logger),
		ErrorHandlerMiddleware(logger),
		SecurityHeadersMiddleware,
		RateLimitMiddleware(limiter, logger),
		MaxSizeMiddleware(50<<20, logger), // 50MB max request size
		TimeoutMiddleware(30*time.Second, logger),
	}
}

// ChainMiddleware chains multiple middleware functions
func ChainMiddleware(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
