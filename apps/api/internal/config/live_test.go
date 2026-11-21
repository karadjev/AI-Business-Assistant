package config

import "testing"

func TestLiveDefaultsAndURLAllowlist(t *testing.T) {
	c, e := LoadLive(func(string) string { return "" })
	if e != nil || c.Ready() || c.PlatformMicroRUB != 0 || c.ExternalAuthorized || !c.Kill || c.DialogInput != 20000 || c.DialogOutput != 5000 || c.DialogRequests != 30 {
		t.Fatal("unsafe live defaults", e)
	}
	allow := []string{"https://provider.example/v1"}
	if e = ValidateBaseURL(allow[0], allow); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"http://provider.example/v1", "https://localhost/v1", "https://127.0.0.1/v1", "https://169.254.169.254/v1", "https://provider.example@127.0.0.1/v1", "https://provider.example/v1?url=x", "https://provider.example:8443/v1", "https://other.example/v1", "https://provider.example/v2"} {
		if ValidateBaseURL(s, append(allow, s)) == nil && s != "https://other.example/v1" {
			t.Fatal("unsafe URL accepted", s)
		}
	}
	if ValidateBaseURL("https://other.example/v1", allow) == nil {
		t.Fatal("not allowlisted host")
	}
}
