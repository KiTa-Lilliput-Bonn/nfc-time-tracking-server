package lanhost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
)

const maxHostLen = 253

var (
	ErrEmpty            = errors.New("android_lan_host darf nicht leer sein")
	ErrTooLong          = errors.New("android_lan_host ist zu lang")
	ErrInvalidChars     = errors.New("android_lan_host enthält ungültige Zeichen (nur IPv4 oder DNS-Name, kein Port)")
	ErrIPv6NotSupported = errors.New("IPv6 wird für android_lan_host nicht unterstützt")
	ErrIPNotAllowed     = errors.New("android_lan_host: nur Loopback- oder private IPv4 (RFC 1918)")
	ErrMetadataBlocked  = errors.New("android_lan_host: diese Adresse ist nicht erlaubt")
	ErrDNS              = errors.New("android_lan_host: DNS-Auflösung fehlgeschlagen")
	ErrDNSNoIPv4        = errors.New("android_lan_host: es wurde keine zulässige IPv4-Adresse gefunden")
	ErrDNSContainsIPv6  = errors.New("android_lan_host: Hostname liefert IPv6 — nicht unterstützt")
)

// ValidateAndroidLANHost prüft den Wert für Einstellung android_lan_host (nur IPv4, kein SSRF auf öffentliche/Metadaten-Ziele).
func ValidateAndroidLANHost(host string) error {
	return ValidateAndroidLANHostContext(context.Background(), host)
}

// ValidateAndroidLANHostContext wie ValidateAndroidLANHost; ctx steuert die DNS-Lookup-Dauer.
func ValidateAndroidLANHostContext(ctx context.Context, host string) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return ErrEmpty
	}
	if len(h) > maxHostLen {
		return ErrTooLong
	}
	if strings.ContainsRune(h, ':') {
		return ErrIPv6NotSupported
	}
	if !hostCharSetOK(h) {
		return ErrInvalidChars
	}

	if ip := net.ParseIP(h); ip != nil {
		return validateIPv4Literal(ip)
	}

	if err := hostNameShapeOK(h); err != nil {
		return err
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, h)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDNS, err)
	}
	if len(addrs) == 0 {
		return ErrDNSNoIPv4
	}
	for _, a := range addrs {
		ip4 := a.IP.To4()
		if ip4 == nil {
			return ErrDNSContainsIPv6
		}
		if err := validateIPv4Literal(ip4); err != nil {
			return err
		}
	}
	return nil
}

func hostCharSetOK(h string) bool {
	for _, r := range h {
		if r == '.' || r == '-' {
			continue
		}
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			continue
		}
		return false
	}
	return true
}

func hostNameShapeOK(h string) error {
	if strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") {
		return ErrInvalidChars
	}
	if strings.Contains(h, "..") {
		return ErrInvalidChars
	}
	parts := strings.Split(h, ".")
	for _, p := range parts {
		if p == "" {
			return ErrInvalidChars
		}
		for _, r := range p {
			if r == '.' {
				return ErrInvalidChars
			}
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
				continue
			}
			return ErrInvalidChars
		}
		if strings.HasPrefix(p, "-") || strings.HasSuffix(p, "-") {
			return ErrInvalidChars
		}
	}
	return nil
}

func validateIPv4Literal(ip net.IP) error {
	ip4 := ip.To4()
	if ip4 == nil {
		return ErrIPv6NotSupported
	}
	if isMetadataIPv4(ip4) {
		return ErrMetadataBlocked
	}
	if !ip4.IsLoopback() && !ip4.IsPrivate() {
		return ErrIPNotAllowed
	}
	return nil
}

// isMetadataIPv4 blockt 169.254.169.0/24 (u.a. Cloud-Metadaten).
func isMetadataIPv4(ip4 net.IP) bool {
	if len(ip4) != 4 {
		return false
	}
	return ip4[0] == 169 && ip4[1] == 254 && ip4[2] == 169
}
