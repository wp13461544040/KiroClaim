package middleware

import (
	"bytes"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	idempotencyHeader = "X-Idempotency-Key"
	idempotencyPrefix = "idem:"
)

type idemRecord struct {
	Status      int
	Body        []byte
	ContentType string
	ExpiresAt   time.Time
}

var (
	idemMu        sync.Mutex
	idemCache     = make(map[string]idemRecord)
	lastIdemSweep time.Time
)

type idemWriter struct {
	gin.ResponseWriter
	buf    *bytes.Buffer
	status int
}

func (w *idemWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *idemWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func loadIdempotentRecord(key string) (idemRecord, bool) {
	now := time.Now()
	idemMu.Lock()
	defer idemMu.Unlock()

	rec, ok := idemCache[key]
	if !ok {
		return idemRecord{}, false
	}
	if now.After(rec.ExpiresAt) {
		delete(idemCache, key)
		return idemRecord{}, false
	}
	return rec, true
}

func storeIdempotentRecord(key string, rec idemRecord, ttl time.Duration) {
	now := time.Now()
	rec.ExpiresAt = now.Add(ttl)

	idemMu.Lock()
	defer idemMu.Unlock()

	idemCache[key] = rec
	if now.Sub(lastIdemSweep) < time.Minute {
		return
	}
	for cachedKey, cached := range idemCache {
		if now.After(cached.ExpiresAt) {
			delete(idemCache, cachedKey)
		}
	}
	lastIdemSweep = now
}

func IdempotencyMiddleware(ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(idempotencyHeader)
		if key == "" {
			c.Next()
			return
		}

		cacheKey := idempotencyPrefix + c.Request.Method + ":" + c.FullPath() + ":" + key
		if rec, ok := loadIdempotentRecord(cacheKey); ok {
			c.Header("Idempotent-Replay", "true")
			c.Data(rec.Status, rec.ContentType, rec.Body)
			c.Abort()
			return
		}

		w := &idemWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}, status: http.StatusOK}
		c.Writer = w
		c.Next()

		if w.status < 200 || w.status >= 300 || w.buf.Len() == 0 {
			return
		}

		body := append([]byte(nil), w.buf.Bytes()...)
		storeIdempotentRecord(cacheKey, idemRecord{
			Status:      w.status,
			Body:        body,
			ContentType: w.Header().Get("Content-Type"),
		}, ttl)
	}
}
