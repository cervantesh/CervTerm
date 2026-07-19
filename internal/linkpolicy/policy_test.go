package linkpolicy

import "testing"

func TestEvaluateRequiresFreshExplicitActivation(t *testing.T) {
	for _, activation := range []Activation{{}, {Explicit: true}, {Fresh: true}} {
		if got := Evaluate("https://example.test/path?token=secret", activation); got.Denial != DeniedActivation || got.URI != "" {
			t.Fatalf("decision=%#v", got)
		}
	}
}

func TestEvaluateCanonicalizesAllowedHTTPWithoutLeakingPayloadLabel(t *testing.T) {
	got := Evaluate("HTTPS://EXAMPLE.TEST:443/a%20b?token=secret#frag", Activation{Explicit: true, Fresh: true})
	if !got.Allowed() || got.URI != "https://example.test:443/a%20b?token=secret#frag" {
		t.Fatalf("decision=%#v", got)
	}
	if got.SafeLabel != "https://example.test:443" {
		t.Fatalf("safe label=%q", got.SafeLabel)
	}
}

func TestEvaluateRejectsSchemesCredentialsAndMalformedURIs(t *testing.T) {
	cases := []struct {
		uri    string
		denial Denial
	}{
		{"file:///etc/passwd", DeniedScheme}, {"mailto:user@example.test", DeniedScheme}, {"javascript:alert(1)", DeniedScheme},
		{"https://user:secret@example.test/x", DeniedAuthority}, {"https:///missing", DeniedAuthority}, {"https://例.example/x", DeniedAuthority}, {"not a url", DeniedMalformed},
	}
	for _, tc := range cases {
		if got := Evaluate(tc.uri, Activation{Explicit: true, Fresh: true}); got.Denial != tc.denial || got.URI != "" {
			t.Errorf("%q => %#v", tc.uri, got)
		}
	}
}

func TestSafeLabelIsBounded(t *testing.T) {
	host := string(make([]byte, 200))
	hostBytes := []byte(host)
	for i := range hostBytes {
		hostBytes[i] = 'a'
	}
	got := Evaluate("https://"+string(hostBytes)+"/private?token=secret", Activation{Explicit: true, Fresh: true})
	if !got.Allowed() || len(got.SafeLabel) > 131 {
		t.Fatalf("decision=%#v labelLen=%d", got, len(got.SafeLabel))
	}
}

func TestEvaluateIPv6AndPortAuthorityRules(t *testing.T) {
	if got := Evaluate("https://[2001:DB8::1]:8443/path", Activation{Explicit: true, Fresh: true}); !got.Allowed() {
		t.Fatalf("IPv6 decision=%#v", got)
	}
	for _, uri := range []string{"https://[fe80::1%25ETH0]/path", "https://[::1]:abc/path", "https://[::1]:65536/path", "https://[::1]:0/path", "https://example.test:/path", "https://example.test:abc/path"} {
		if got := Evaluate(uri, Activation{Explicit: true, Fresh: true}); got.Allowed() {
			t.Errorf("%q allowed: %#v", uri, got)
		}
	}
}
