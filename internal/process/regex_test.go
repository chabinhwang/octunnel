package process

import "testing"

func TestOpencodeListenRe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"standard http",
			"listening on http://127.0.0.1:3000",
			"http://127.0.0.1:3000",
		},
		{
			"https",
			"listening on https://localhost:8443",
			"https://localhost:8443",
		},
		{
			"with trailing text",
			"INFO listening on http://0.0.0.0:4321 (press Ctrl+C to stop)",
			"http://0.0.0.0:4321",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := OpencodeListenRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.want {
				t.Errorf("got %q, want %q", m[1], tt.want)
			}
		})
	}
}

func TestQuickTunnelURLRe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"standard URL",
			"Your quick Tunnel has been created! Visit it at https://random-words-here.trycloudflare.com",
			"https://random-words-here.trycloudflare.com",
		},
		{
			"with numbers",
			"https://abc-123-def.trycloudflare.com is ready",
			"https://abc-123-def.trycloudflare.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := QuickTunnelURLRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.want {
				t.Errorf("got %q, want %q", m[1], tt.want)
			}
		})
	}
}

func TestCertPemRe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"standard path",
			"You have successfully logged in. The certificate has been saved to: /home/user/.cloudflared/cert.pem",
			"/home/user/.cloudflared/cert.pem",
		},
		{
			"macOS path",
			"saved to: /Users/john/.cloudflared/cert.pem",
			"/Users/john/.cloudflared/cert.pem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := CertPemRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.want {
				t.Errorf("got %q, want %q", m[1], tt.want)
			}
		})
	}
}

func TestTunnelCreatedRe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantID   string
	}{
		{
			"standard output",
			"Created tunnel octunnel with id a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"octunnel",
			"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
		{
			"numbered name",
			"Created tunnel octunnel3 with id 12345678-abcd-ef01-2345-678901234567",
			"octunnel3",
			"12345678-abcd-ef01-2345-678901234567",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := TunnelCreatedRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.wantName {
				t.Errorf("name: got %q, want %q", m[1], tt.wantName)
			}
			if m[2] != tt.wantID {
				t.Errorf("id: got %q, want %q", m[2], tt.wantID)
			}
		})
	}
}

func TestTunnelCredFileRe(t *testing.T) {
	input := "Tunnel credentials written to /home/user/.cloudflared/a1b2c3d4.json"
	m := TunnelCredFileRe.FindStringSubmatch(input)
	if m == nil {
		t.Fatalf("no match")
	}
	want := "/home/user/.cloudflared/a1b2c3d4.json"
	if m[1] != want {
		t.Errorf("got %q, want %q", m[1], want)
	}
}

func TestTunnelExistsRe(t *testing.T) {
	input := "failed to create tunnel: tunnel with name already exists"
	if !TunnelExistsRe.MatchString(input) {
		t.Errorf("expected match for %q", input)
	}

	if TunnelExistsRe.MatchString("Created tunnel octunnel with id abc-123") {
		t.Errorf("should not match successful creation output")
	}
}

func TestNoFalsePositives(t *testing.T) {
	irrelevant := []string{
		"INFO Starting cloudflared",
		"Connection established",
		"Registered tunnel connection",
		"",
	}
	patterns := []struct {
		name string
		re   interface{ MatchString(string) bool }
	}{
		{"OpencodeListenRe", OpencodeListenRe},
		{"QuickTunnelURLRe", QuickTunnelURLRe},
		{"CertPemRe", CertPemRe},
		{"TunnelCreatedRe", TunnelCreatedRe},
		{"TunnelCredFileRe", TunnelCredFileRe},
		{"TunnelExistsRe", TunnelExistsRe},
	}

	for _, p := range patterns {
		for _, line := range irrelevant {
			if p.re.MatchString(line) {
				t.Errorf("%s should not match %q", p.name, line)
			}
		}
	}
}
