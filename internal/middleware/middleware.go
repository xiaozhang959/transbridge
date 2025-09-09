// internal/middleware/middleware.go
package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MiddlewareFunc 定义中间件函数类型
type MiddlewareFunc func(http.HandlerFunc) http.HandlerFunc

// Chain 将多个中间件串联起来
func Chain(handler http.HandlerFunc, middlewares ...MiddlewareFunc) http.HandlerFunc {
	// 从后往前包装处理函数
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// Logger 是一个记录HTTP请求日志的中间件
func Logger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 包装ResponseWriter以捕获状态码
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// 处理请求
		next.ServeHTTP(wrapped, r)

		// 计算处理时间
		duration := time.Since(start)

		// 记录请求信息
		log.Printf(
			"Method: %s | Path: %s | Status: %d | Duration: %v | IP: %s | User-Agent: %s",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			duration,
			r.RemoteAddr,
			r.UserAgent(),
		)
	}
}

// CORS 处理跨域请求的中间件
func CORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 设置CORS头（可根据需要限制域名）
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// 处理预检请求
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// RateLimiter 实现简单的速率限制中间件
func RateLimiter(requestsPerMinute int) MiddlewareFunc {
	// 创建令牌桶，使用互斥锁保护
	var mu sync.Mutex
	bucket := make(map[string][]time.Time)

	// 定期清理过期的记录
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			mu.Lock()
			now := time.Now()
			windowStart := now.Add(-time.Minute)
			for ip, times := range bucket {
				var validTimes []time.Time
				for _, t := range times {
					if t.After(windowStart) {
						validTimes = append(validTimes, t)
					}
				}
				if len(validTimes) > 0 {
					bucket[ip] = validTimes
				} else {
					delete(bucket, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			clientIP := realIP(r)

			mu.Lock()
			times := bucket[clientIP]
			now := time.Now()
			windowStart := now.Add(-time.Minute)

			var validTimes []time.Time
			for _, t := range times {
				if t.After(windowStart) {
					validTimes = append(validTimes, t)
				}
			}

			if len(validTimes) >= requestsPerMinute {
				mu.Unlock()
				http.Error(w, "Too many requests", http.StatusTooManyRequests)
				return
			}

			bucket[clientIP] = append(validTimes, now)
			mu.Unlock()

			next.ServeHTTP(w, r)
		}
	}
}

// realIP 尝试从代理头中解析真实客户端 IP
func realIP(r *http.Request) string {
	// 优先 X-Forwarded-For（取第一个）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if parsed := net.ParseIP(ip); parsed != nil {
			return ip
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		if parsed := net.ParseIP(xrip); parsed != nil {
			return xrip
		}
	}
	// 回退到 RemoteAddr（去掉端口）
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// Recovery 是一个恢复panic的中间件
func Recovery(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	}
}

// responseWriter 包装了http.ResponseWriter以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write 实现 http.ResponseWriter 接口
func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

// Flush 实现 http.Flusher 接口
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
