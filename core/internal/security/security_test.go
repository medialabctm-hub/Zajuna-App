package security

import (
	"strings"
	"testing"
)

func TestRedactURLRemovesCredentialQueryParameters(t *testing.T) {
	value := RedactURL("https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080&sesskey=secret&section=2#fragment")
	if value != "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080&section=2" {
		t.Fatalf("unexpected redacted URL: %s", value)
	}
}

func TestRedactTextRemovesCredentialForms(t *testing.T) {
	value := RedactText("Authorization: Bearer abcdefghijkl; Cookie: MoodleSession=s3cr3t; url?sesskey=k1&id=7 password = p4ss")
	for _, secret := range []string{"abcdefghijkl", "s3cr3t", "k1", "p4ss"} {
		if strings.Contains(value, secret) {
			t.Fatalf("RedactText leaked %q: %s", secret, value)
		}
	}
	if !strings.Contains(value, "id=7") {
		t.Fatalf("functional parameters must survive: %s", value)
	}
}

func TestValidateHTTPURLRejectsPrivateAndUnapprovedOrigins(t *testing.T) {
	if _, err := ValidateHTTPURL("http://127.0.0.1:8080/debug", []string{"https://zajuna.sena.edu.co"}, false); err == nil {
		t.Fatal("expected private URL rejection")
	}
	if _, err := ValidateHTTPURL("https://example.com/", []string{"https://zajuna.sena.edu.co"}, false); err == nil {
		t.Fatal("expected unapproved origin rejection")
	}
}

func TestValidateHTTPURLAllowsFixtureWhenExplicitlyEnabled(t *testing.T) {
	if _, err := ValidateHTTPURL("http://127.0.0.1:8080/fixture", nil, true); err != nil {
		t.Fatalf("expected fixture URL to be allowed: %v", err)
	}
}
