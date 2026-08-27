package api

import "testing"

func TestNormalizeNTPServer(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"bare hostname", "0.th.pool.ntp.org", "0.th.pool.ntp.org", false},
		{"bare IP", "192.168.1.1", "192.168.1.1", false},
		{"empty disables sync", "", "", false},
		{"whitespace-only treated as empty", "   ", "", false},
		{"strips ntp.conf server prefix", "server 0.th.pool.ntp.org", "0.th.pool.ntp.org", false},
		{"strips ntp.conf pool prefix", "pool 0.th.pool.ntp.org", "0.th.pool.ntp.org", false},
		{"strips leading/trailing whitespace", "  0.th.pool.ntp.org  ", "0.th.pool.ntp.org", false},
		{"prefix strip is case-insensitive", "Server 0.th.pool.ntp.org", "0.th.pool.ntp.org", false},
		{"embedded whitespace after strip rejected", "server 0.th.pool.ntp.org extra", "", true},
		{"embedded whitespace with no prefix rejected", "not a hostname", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeNTPServer(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeNTPServer(%q) = %q, nil; want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeNTPServer(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeNTPServer(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
