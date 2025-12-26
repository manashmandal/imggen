package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

var (
	allowedHosts = []string{
		"oaidalleapiprodscus.blob.core.windows.net",
		"dalleprodsec.blob.core.windows.net",
	}

	ErrPrivateIP     = fmt.Errorf("URL resolves to private IP address")
	ErrUntrustedHost = fmt.Errorf("URL host is not trusted")
	ErrInvalidScheme = fmt.Errorf("only HTTPS URLs are allowed")

	skipValidation = false
)

func SetSkipValidation(skip bool) {
	skipValidation = skip
}

func ValidateImageURL(rawURL string, strictMode bool) error {
	if skipValidation {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return ErrInvalidScheme
	}

	host := parsed.Hostname()

	if strictMode && !isAllowedHost(host) {
		return ErrUntrustedHost
	}

	return validateHostIP(host)
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
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return ErrPrivateIP
		}
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
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
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 0: // 0.0.0.0/8
			return true
		case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127: // 100.64.0.0/10 (CGNAT)
			return true
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0: // 192.0.0.0/24
			return true
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2: // 192.0.2.0/24 (TEST-NET-1)
			return true
		case ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100: // 198.51.100.0/24 (TEST-NET-2)
			return true
		case ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113: // 203.0.113.0/24 (TEST-NET-3)
			return true
		case ip4[0] >= 224 && ip4[0] <= 239: // 224.0.0.0/4 (Multicast)
			return true
		case ip4[0] >= 240: // 240.0.0.0/4 (Reserved)
			return true
		}
	}

	return false
}
