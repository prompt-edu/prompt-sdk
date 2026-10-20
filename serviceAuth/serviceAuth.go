package serviceAuth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/utils"
)

const serviceContextKey = "authenticatedServiceName"

// Keys maps a service name to its accepted shared secrets. A service may have
// more than one so a key can be rotated without downtime.
type Keys map[string][]string

// ParseKeys parses "service1:key1,service2:key2", with a service repeated to
// list several keys. Empty keys are dropped: subtle.ConstantTimeCompare("", "")
// returns 1, so a blank key would authenticate every request for that service.
func ParseKeys(raw string) Keys {
	keys := make(Keys)
	for _, pair := range strings.Split(raw, ",") {
		service, key, ok := strings.Cut(strings.TrimSpace(pair), ":")
		service, key = strings.TrimSpace(service), strings.TrimSpace(key)
		if !ok || service == "" || key == "" {
			continue
		}
		keys[service] = append(keys[service], key)
	}
	return keys
}

// Middleware admits a request whose serviceHeader names a configured service
// and whose tokenHeader carries one of that service's keys. The token header is
// removed on success, so nothing that logs the request later can leak it.
func Middleware(keys Keys, serviceHeader, tokenHeader string) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := c.GetHeader(serviceHeader)
		if service == "" || !matchesAnyKey(c.GetHeader(tokenHeader), keys[service]) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, utils.ErrorResponse{Error: "invalid service credentials"})
			return
		}
		c.Request.Header.Del(tokenHeader)
		c.Set(serviceContextKey, service)
		c.Next()
	}
}

// ServiceName returns the service that Middleware authenticated.
func ServiceName(c *gin.Context) string {
	return c.GetString(serviceContextKey)
}

// matchesAnyKey compares SHA-256 digests against every key without
// short-circuiting, so its timing leaks neither a key's length nor which key matched.
func matchesAnyKey(token string, accepted []string) bool {
	tokenDigest := sha256.Sum256([]byte(token))
	match := false
	for _, key := range accepted {
		keyDigest := sha256.Sum256([]byte(key))
		if subtle.ConstantTimeCompare(tokenDigest[:], keyDigest[:]) == 1 {
			match = true
		}
	}
	return match
}
