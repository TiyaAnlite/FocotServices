package main

import "testing"

func TestParseProbeTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "https default port",
			endpoint: "https://bilibili.com",
			wantHost: "bilibili.com",
			wantPort: 443,
		},
		{
			name:     "https explicit port and path ignored",
			endpoint: "https://bilibili.com:8443/path?q=1#part",
			wantHost: "bilibili.com",
			wantPort: 8443,
		},
		{
			name:     "http default port",
			endpoint: "http://bilibili.com/live",
			wantHost: "bilibili.com",
			wantPort: 80,
		},
		{
			name:     "unsupported scheme",
			endpoint: "tcp://bilibili.com:443",
			wantErr:  true,
		},
		{
			name:     "missing host",
			endpoint: "https:///path",
			wantErr:  true,
		},
		{
			name:     "invalid port",
			endpoint: "https://bilibili.com:70000",
			wantErr:  true,
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			target, err := ParseProbeTarget(testCase.endpoint)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("expected error for endpoint %q", testCase.endpoint)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseProbeTarget returned error: %v", err)
			}
			if target.Host != testCase.wantHost {
				t.Fatalf("host mismatch, got %q want %q", target.Host, testCase.wantHost)
			}
			if target.Port != testCase.wantPort {
				t.Fatalf("port mismatch, got %d want %d", target.Port, testCase.wantPort)
			}
		})
	}
}
