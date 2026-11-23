package urlcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsecureExternal(t *testing.T) {
	for in, want := range map[string]bool{
		"https://core.example.com": false, // TLS
		"http://server-core:8080":  false, // internal docker service name
		"http://localhost:8080":    false,
		"http://127.0.0.1:8080":    false,
		"http://10.0.0.5:8080":     false, // private IP
		"http://192.168.1.10":      false, // private IP
		"http://core.example.com":  true,  // public FQDN over plaintext
		"http://8.8.8.8:8080":      true,  // public IP over plaintext
		"server-core:8080":         true,  // scheme-less: parses with no host, warn
		"":                         true,  // empty: malformed config, warn
	} {
		assert.Equal(t, want, InsecureExternal(in), in)
	}
}
