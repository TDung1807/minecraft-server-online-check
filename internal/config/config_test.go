package config

import "testing"

func TestDefaults(t *testing.T) {
	for _, key := range []string{"MC_HOST", "MC_PORT", "POLL_INTERVAL", "QUERY_TIMEOUT", "STALE_AFTER", "HTTP_ADDR", "METRICS_ADDR", "WEB_ORIGIN", "TRUST_PROXY"} {
		t.Setenv(key, "")
	}
	c, err := Load()
	if err != nil || c.Host != "bora.pikamc.vn" || c.Port != 25005 {
		t.Fatal(c, err)
	}
}
func TestInvalid(t *testing.T) {
	for _, tc := range []struct{ key, value string }{{"MC_PORT", "0"}, {"MC_HOST", "http://example.com"}, {"POLL_INTERVAL", "0s"}, {"QUERY_TIMEOUT", "6s"}, {"STALE_AFTER", "2s"}, {"HTTP_ADDR", "bad"}, {"WEB_ORIGIN", "https://example.com/path"}, {"TRUST_PROXY", "yes"}} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}
