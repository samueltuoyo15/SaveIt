package utils

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	if ip := ClientIP(req); ip != "192.168.1.1" {
		t.Errorf("got %q, want 192.168.1.1", ip)
	}

	req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
	if ip := ClientIP(req); ip != "203.0.113.195" {
		t.Errorf("got %q, want 203.0.113.195", ip)
	}

	req.Header.Set("CF-Connecting-IP", "198.51.100.1")
	if ip := ClientIP(req); ip != "198.51.100.1" {
		t.Errorf("got %q, want 198.51.100.1", ip)
	}
}
