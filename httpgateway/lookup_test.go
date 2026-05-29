package httpgateway

import (
	"net/http/httptest"
	"testing"
)

func TestRequestSourceIPUsesForwardedHeadersFromTrustedProxy(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		value    string
		remote   string
		expected string
	}{
		{
			name:     "x forwarded for",
			header:   "X-Forwarded-For",
			value:    "203.0.113.10, 192.168.0.12",
			remote:   "192.168.0.12:443",
			expected: "203.0.113.10",
		},
		{
			name:     "x real ip",
			header:   "X-Real-IP",
			value:    "198.51.100.23",
			remote:   "127.0.0.1:8080",
			expected: "198.51.100.23",
		},
		{
			name:     "forwarded header",
			header:   "Forwarded",
			value:    `for="198.51.100.45:1234";proto=https`,
			remote:   "10.0.0.8:443",
			expected: "198.51.100.45",
		},
		{
			name:     "ipv6 forwarded header",
			header:   "Forwarded",
			value:    `for="[2001:db8::1]:1234";proto=https`,
			remote:   "172.16.0.8:443",
			expected: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com", nil)
			req.RemoteAddr = tt.remote
			req.Header.Set(tt.header, tt.value)

			if got := requestSourceIP(req); got != tt.expected {
				t.Fatalf("requestSourceIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRequestSourceIPIgnoresSpoofedForwardedHeadersFromPublicRemote(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "203.0.113.20:443"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")

	if got := requestSourceIP(req); got != "203.0.113.20" {
		t.Fatalf("requestSourceIP() = %q, want remote address", got)
	}
}
