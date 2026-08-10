package buildinfo

import "testing"

func TestShortVersion(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", "dev"},
		{"pure semver", "v1.2.3", "v1.2.3"},
		{"semver with suffix", "v1.2.3-enterprise", "v1.2.3-enterprise"},
		{"semver embedded", "vraxel-server-v1.0.0-enterprise-...", "v1.0.0-enterprise"},
		{"Makefile dev build",
			"vraxel-server-20260513-061329-heads-local-main-0-g470c9a01",
			"20260513-g470c9a01"},
		{"Makefile build with branch suffix",
			"vraxel-server-20260101-000000-tags-v0-1-0-0-gabcdef12",
			"20260101-gabcdef12"},
		{"unrecognized", "totally-custom", "totally-custom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := Version
			Version = tc.in
			defer func() { Version = old }()
			if got := ShortVersion(); got != tc.want {
				t.Errorf("ShortVersion()=%q, want %q (Version=%q)", got, tc.want, tc.in)
			}
		})
	}
}
