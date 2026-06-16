package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplyInsecureTLSSetsDefaultTransport(t *testing.T) {
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })

	ApplyInsecureTLS()

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", http.DefaultTransport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify=true in TLS config")
	}
}

func TestInsecureClientReachesTLSServer(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })

	client := &http.Client{}

	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("expected TLS error with default transport, got nil")
	}

	ApplyInsecureTLS()

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed after ApplyInsecureTLS: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
