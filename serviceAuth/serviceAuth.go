package serviceAuth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/utils"
	log "github.com/sirupsen/logrus"
)

const serviceContextKey = "authenticatedServiceName"

// Keys maps a service name to its accepted shared secrets. A service may have
// more than one so a key can be rotated without downtime.
type Keys map[string][]string

// ParseKeys parses "service1:key1,service2:key2", with a service repeated to
// list several keys. Empty keys are dropped: subtle.ConstantTimeCompare("", "")
// returns 1, so a blank key would authenticate every request for that service.
// Every dropped entry and every key listed under more than one service is
// logged as a warning that names the entry's position and service, never a key.
func ParseKeys(raw string) Keys {
	keys := make(Keys)
	serviceOfKey := make(map[string]string)
	for index, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		service, key, ok := strings.Cut(pair, ":")
		service, key = strings.TrimSpace(service), strings.TrimSpace(key)
		switch {
		case !ok:
			log.Warnf("serviceAuth: skipping entry %d, which is not in the form service:key", index+1)
			continue
		case service == "":
			log.Warnf("serviceAuth: skipping entry %d, which names no service", index+1)
			continue
		case key == "":
			log.Warnf("serviceAuth: skipping entry %d for service %q, which has an empty key", index+1, service)
			continue
		}
		if other, seen := serviceOfKey[key]; seen && other != service {
			log.Warnf("serviceAuth: services %q and %q share a key, so either can authenticate as the other", other, service)
		} else {
			serviceOfKey[key] = service
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
		service, token := c.GetHeader(serviceHeader), c.GetHeader(tokenHeader)
		if service == "" || token == "" || !matchesAnyKey(token, keys[service]) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, utils.ErrorResponse{Error: "invalid service credentials"})
			return
		}
		c.Request.Header.Del(tokenHeader)
		c.Set(serviceContextKey, service)
		c.Next()
	}
}

// ServiceName returns the service that Middleware authenticated. It reports false
// when the route runs without Middleware, so a handler never acts for an empty service.
func ServiceName(c *gin.Context) (string, bool) {
	service := c.GetString(serviceContextKey)
	return service, service != ""
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
