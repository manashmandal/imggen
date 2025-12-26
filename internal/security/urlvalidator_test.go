package security

import (
	"net"
	"testing"
)

func TestValidateImageURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		strictMode bool
		wantErr    error
	}{
		{
			name:       "valid OpenAI blob URL",
			url:        "https://oaidalleapiprodscus.blob.core.windows.net/image.png",
			strictMode: true,
			wantErr:    nil,
		},
		{
			name:       "valid other HTTPS URL in non-strict mode",
			url:        "https://example.com/image.png",
			strictMode: false,
			wantErr:    nil,
		},
		{
			name:       "untrusted host in strict mode",
			url:        "https://example.com/image.png",
			strictMode: true,
			wantErr:    ErrUntrustedHost,
		},
		{
			name:       "HTTP URL rejected",
			url:        "http://oaidalleapiprodscus.blob.core.windows.net/image.png",
			strictMode: false,
			wantErr:    ErrInvalidScheme,
		},
		{
			name:       "localhost rejected",
			url:        "https://localhost/image.png",
			strictMode: false,
			wantErr:    ErrPrivateIP,
		},
		{
			name:       "127.0.0.1 rejected",
			url:        "https://127.0.0.1/image.png",
			strictMode: false,
			wantErr:    ErrPrivateIP,
		},
		{
			name:       "private IP 10.x rejected",
			url:        "https://10.0.0.1/image.png",
			strictMode: false,
			wantErr:    ErrPrivateIP,
		},
		{
			name:       "private IP 172.16.x rejected",
			url:        "https://172.16.0.1/image.png",
			strictMode: false,
			wantErr:    ErrPrivateIP,
		},
		{
			name:       "private IP 192.168.x rejected",
			url:        "https://192.168.1.1/image.png",
			strictMode: false,
			wantErr:    ErrPrivateIP,
		},
		{
			name:       "link-local 169.254.x rejected",
			url:        "https://169.254.169.254/image.png",
			strictMode: false,
			wantErr:    ErrPrivateIP,
		},
		{
			name:       "IPv6 loopback rejected",
			url:        "https://[::1]/image.png",
			strictMode: false,
			wantErr:    ErrPrivateIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageURL(tt.url, tt.strictMode)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateImageURL() error = %v, wantErr nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateImageURL() error = nil, wantErr %v", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("ValidateImageURL() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"169.254.169.254", true},
		{"0.0.0.0", true},
		{"100.64.0.1", true},    // Carrier-grade NAT
		{"192.0.2.1", true},     // TEST-NET-1
		{"198.51.100.1", true},  // TEST-NET-2
		{"203.0.113.1", true},   // TEST-NET-3
		{"224.0.0.1", true},     // Multicast
		{"240.0.0.1", true},     // Reserved
		{"8.8.8.8", false},      // Google DNS
		{"1.1.1.1", false},      // Cloudflare
		{"20.150.38.228", false}, // Azure blob storage
		{"::1", true},           // IPv6 loopback
		{"fe80::1", true},       // IPv6 link-local
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := parseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func parseIP(s string) net.IP {
	return net.ParseIP(s)
}
