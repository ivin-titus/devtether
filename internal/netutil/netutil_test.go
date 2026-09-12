package netutil

import (
	"testing"
)

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// 1. Empty strings
		{"empty string", "", ""},
		{"spaces only", "   ", ""},
		
		// 2. Standard domains
		{"standard domain", "example.com", "example.com"},
		{"standard domain with port", "example.com:8080", "example.com"},
		{"standard domain with very large port", "example.com:65535", "example.com"},
		
		// 3. Trailing dots
		{"trailing dot", "example.com.", "example.com"},
		{"trailing dot with port", "example.com.:8080", "example.com"},
		
		// 4. Mixed casing
		{"mixed casing", "ExAmPlE.com", "example.com"},
		{"mixed casing with port", "ExAmPlE.com:8080", "example.com"},
		{"mixed casing with trailing dot", "ExAmPlE.com.", "example.com"},
		
		// 5. Bare IPv6
		{"bare IPv6", "::1", "::1"},
		{"bare IPv6 in brackets", "[::1]", "::1"},
		{"bare IPv6 full", "2001:db8:85a3:8d3:1319:8a2e:370:7348", "2001:db8:85a3:8d3:1319:8a2e:370:7348"},
		
		// 6. IPv6 with ports
		{"IPv6 with port", "[::1]:8080", "::1"},
		{"IPv6 full with port", "[2001:db8:85a3:8d3:1319:8a2e:370:7348]:443", "2001:db8:85a3:8d3:1319:8a2e:370:7348"},
		
		// 7. Malformed IPv6 edge cases
		{"malformed IPv6 missing closing bracket", "[localhost", "[localhost"},
		{"malformed IPv6 missing opening bracket", "::1]", "::1]"},
		{"malformed IPv6 with port missing closing bracket", "[::1:8080", ""},
		{"malformed IPv6 extra colons with bracket", "[2001:db8::1:8080", ""},
		{"garbage port", "example.com:abc", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeHost(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeHost(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}
