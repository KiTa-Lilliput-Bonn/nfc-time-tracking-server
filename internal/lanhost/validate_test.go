package lanhost

import (
	"context"
	"errors"
	"testing"
)

func TestValidateAndroidLANHost_IPv4Literals(t *testing.T) {
	t.Parallel()
	good := []string{"192.168.1.1", "10.0.0.1", "127.0.0.1", "  172.16.0.2  "}
	for _, h := range good {
		if err := ValidateAndroidLANHost(h); err != nil {
			t.Errorf("expected ok for %q, got %v", h, err)
		}
	}
	bad := []struct {
		host string
		want error
	}{
		{"8.8.8.8", ErrIPNotAllowed},
		{"100.64.0.1", ErrIPNotAllowed},
		{"169.254.1.1", ErrIPNotAllowed},
		{"169.254.169.254", ErrMetadataBlocked},
		{"169.254.169.1", ErrMetadataBlocked},
		{"0.0.0.0", ErrIPNotAllowed},
	}
	for _, tc := range bad {
		err := ValidateAndroidLANHost(tc.host)
		if !errors.Is(err, tc.want) {
			t.Errorf("host %q: want %v, got %v", tc.host, tc.want, err)
		}
	}
}

func TestValidateAndroidLANHost_IPv6Rejected(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"::1", "fe80::1", "2001:db8::1"} {
		err := ValidateAndroidLANHost(h)
		if !errors.Is(err, ErrIPv6NotSupported) {
			t.Errorf("host %q: want ErrIPv6NotSupported, got %v", h, err)
		}
	}
}

func TestValidateAndroidLANHost_InvalidShape(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"192.168.1.1:8080",
		"../evil",
		"host space",
		".leading",
		"trailing.",
		"double..dot",
		"-bad.com",
		"bad-.com",
	}
	for _, h := range cases {
		err := ValidateAndroidLANHost(h)
		if err == nil {
			t.Errorf("expected error for %q", h)
		}
	}
}

func TestValidateAndroidLANHost_DNS(t *testing.T) {
	t.Parallel()
	// localhost löst typischerweise zu 127.0.0.1 auf (IPv4).
	ctx := context.Background()
	if err := ValidateAndroidLANHostContext(ctx, "localhost"); err != nil {
		t.Skipf("localhost DNS not usable in this environment: %v", err)
	}
}
