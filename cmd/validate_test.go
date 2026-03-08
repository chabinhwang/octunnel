package cmd

import "testing"

func TestValidateSubdomain(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		// valid
		{"open", false},
		{"my-app", false},
		{"a", false},
		{"a1", false},
		{"opencode-server-1", false},
		{"123", false},
		{"a-b-c", false},

		// empty / whitespace
		{"", true},

		// uppercase (caller lowercases, but validator rejects)
		{"Open", true},
		{"MY-APP", true},

		// invalid characters
		{"my_app", true},
		{"my app", true},
		{"my.app", true},
		{"hello!", true},
		{"서버", true},
		{"my@app", true},

		// starts/ends with hyphen
		{"-start", true},
		{"end-", true},
		{"-both-", true},

		// too long (64 chars)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},

		// max valid length (63 chars)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validateSubdomain(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSubdomain(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		// valid
		{"example.com", false},
		{"my-site.co.kr", false},
		{"sub.example.com", false},
		{"a.io", false},
		{"deep.nested.sub.example.com", false},
		{"123.example.com", false},

		// empty
		{"", true},

		// no TLD
		{"localhost", true},
		{"example", true},

		// single-char TLD
		{"example.a", true},

		// invalid characters
		{"exam ple.com", true},
		{"example_.com", true},
		{"example!.com", true},
		{"서버.com", true},

		// starts/ends with hyphen in label
		{"-example.com", true},
		{"example-.com", true},

		// numeric TLD (not valid — TLDs are alpha)
		{"example.123", true},

		// trailing dot
		{"example.com.", true},

		// protocol (should be stripped before calling)
		{"https://example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validateDomain(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDomain(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
