package linkpolicy

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const MaxURIBytes = 2048

type Activation struct {
	Explicit bool
	Fresh    bool
}

type Denial string

const (
	DeniedNone       Denial = ""
	DeniedActivation Denial = "activation-required"
	DeniedMalformed  Denial = "malformed-uri"
	DeniedScheme     Denial = "scheme-not-allowed"
	DeniedAuthority  Denial = "invalid-authority"
)

type Decision struct {
	URI       string
	SafeLabel string
	Denial    Denial
}

func Evaluate(raw string, activation Activation) Decision {
	if !activation.Explicit || !activation.Fresh {
		return Decision{SafeLabel: "enlace", Denial: DeniedActivation}
	}
	if raw == "" || len(raw) > MaxURIBytes {
		return Decision{SafeLabel: "enlace", Denial: DeniedMalformed}
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" {
		return Decision{SafeLabel: "enlace", Denial: DeniedMalformed}
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Decision{SafeLabel: safeSchemeLabel(parsed.Scheme), Denial: DeniedScheme}
	}
	if parsed.Opaque != "" {
		return Decision{SafeLabel: safeSchemeLabel(parsed.Scheme), Denial: DeniedMalformed}
	}
	hostname := parsed.Hostname()
	if parsed.User != nil || parsed.Host == "" || hostname == "" || !asciiHost(hostname) || strings.Contains(hostname, "%") {
		return Decision{SafeLabel: safeSchemeLabel(parsed.Scheme), Denial: DeniedAuthority}
	}
	port := parsed.Port()
	if !validAuthorityPort(parsed.Host, port) {
		return Decision{SafeLabel: safeSchemeLabel(parsed.Scheme), Denial: DeniedAuthority}
	}
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return Decision{SafeLabel: safeSchemeLabel(parsed.Scheme), Denial: DeniedAuthority}
		}
	}
	if net.ParseIP(hostname) == nil {
		hostname = strings.ToLower(hostname)
		parsed.Host = hostname
		if port != "" {
			parsed.Host = net.JoinHostPort(hostname, port)
		}
	}
	return Decision{URI: parsed.String(), SafeLabel: safeAuthorityLabel(parsed.Scheme, parsed.Host)}
}

func (d Decision) Allowed() bool { return d.Denial == DeniedNone && d.URI != "" }

func (d Decision) Error() error {
	if d.Allowed() {
		return nil
	}
	return errors.New(string(d.Denial))
}

func safeAuthorityLabel(scheme, host string) string {
	const max = 128
	label := scheme + "://" + host
	if len(label) > max {
		return label[:max] + "…"
	}
	return label
}

func validAuthorityPort(host, port string) bool {
	if strings.HasPrefix(host, "[") {
		close := strings.LastIndex(host, "]")
		if close < 0 {
			return false
		}
		suffix := host[close+1:]
		return suffix == "" || (strings.HasPrefix(suffix, ":") && len(suffix) > 1 && port != "")
	}
	return !strings.Contains(host, ":") || port != ""
}

func safeSchemeLabel(scheme string) string {
	if scheme == "" {
		return "enlace"
	}
	if len(scheme) > 32 {
		scheme = scheme[:32]
	}
	return scheme + ":…"
}

func asciiHost(host string) bool {
	for _, r := range host {
		if r > 0x7f || r <= 0x20 {
			return false
		}
	}
	return true
}
