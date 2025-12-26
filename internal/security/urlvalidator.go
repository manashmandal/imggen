package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

var (
	// allowedHosts contains trusted hosts for image downloads
	allowedHosts = []string{
		"oaidalleapiprodscus.blob.core.windows.net",
		"dalleprodsec.blob.core.windows.net",
	}

	// ErrPrivateIP is returned when URL resolves to a private IP
	ErrPrivateIP = fmt.Errorf("URL resolves to private IP address")

	// ErrUntrustedHost is returned when URL host is not in allowlist
	ErrUntrustedHost = fmt.Errorf("URL host is not trusted")

	// ErrInvalidScheme is returned for non-HTTPS URLs
	ErrInvalidScheme = fmt.Errorf("only HTTPS URLs are allowed")

	// skipValidation is used for testing purposes only
	skipValidation = false
)

// SetSkipValidation enables/disables URL validation (for testing only)
func SetSkipValidation(skip bool) {
	skipValidation = skip
}

// ValidateImageURL validates that a URL is safe to fetch
// It checks for:
// - HTTPS scheme only
// - Host is in allowlist (if strict mode)
// - Does not resolve to private IP ranges
func ValidateImageURL(rawURL string, strictMode bool) error {
	// Skip validation in test mode
	if skipValidation {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow HTTPS
	if parsed.Scheme != "https" {
		return ErrInvalidScheme
	}

	host := parsed.Hostname()

	// In strict mode, only allow known OpenAI blob storage hosts
	if strictMode {
		if !isAllowedHost(host) {
			return ErrUntrustedHost
		}
	}

	// Check if host resolves to private IP
	if err := validateHostIP(host); err != nil {
		return err
	}

	return nil
}

func isAllowedHost(host string) bool {
	host = strings.ToLower(host)
	for _, allowed := range allowedHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func validateHostIP(host string) error {
	// Try to parse as IP first
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return ErrPrivateIP
		}
		return nil
	}

	// Resolve hostname
	ips, err := net.LookupIP(host)
	if err != nil {
		// Allow if we can't resolve - let the HTTP client handle it
		return nil
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return ErrPrivateIP
		}
	}

	return nil
}

func isPrivateIP(ip net.IP) bool {
	// Check for loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local (169.254.0.0/16, fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private ranges
	if ip.IsPrivate() {
		return true
	}

	// Check for unspecified (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return true
	}

	// Additional checks for IPv4 special ranges
	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8 - Current network
		if ip4[0] == 0 {
			return true
		}
		// 100.64.0.0/10 - Carrier-grade NAT
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		// 192.0.0.0/24 - IETF Protocol Assignments
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0 {
			return true
		}
		// 192.0.2.0/24 - TEST-NET-1
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
			return true
		}
		// 198.51.100.0/24 - TEST-NET-2
		if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
			return true
		}
		// 203.0.113.0/24 - TEST-NET-3
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return true
		}
		// 224.0.0.0/4 - Multicast
		if ip4[0] >= 224 && ip4[0] <= 239 {
			return true
		}
		// 240.0.0.0/4 - Reserved
		if ip4[0] >= 240 {
			return true
		}
	}

	return false
}
