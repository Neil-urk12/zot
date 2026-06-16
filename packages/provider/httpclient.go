package provider

import (
	"crypto/tls"
	"net/http"
)

// InsecureSkipVerify disables TLS cert verification for inference connections.
var InsecureSkipVerify bool

// ApplyInsecureTLS replaces http.DefaultTransport to skip TLS cert verification.
func ApplyInsecureTLS() {
	http.DefaultTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
}
