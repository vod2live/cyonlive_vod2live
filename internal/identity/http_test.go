package identity

import "testing"

func TestIdentityInputValidation(t *testing.T) {
	if !validEmail("admin@example.com") || validEmail("Not an email") || validEmail("Display Name <admin@example.com>") {
		t.Fatal("email validation accepted or rejected an unexpected value")
	}
	if !validPassword("long-enough-password") || validPassword("short") || validPassword(string(make([]byte, 73))) {
		t.Fatal("password length validation failed")
	}
	if got := slugify("Minha Organização 2026"); got != "minha-organiza-o-2026" {
		t.Fatalf("unexpected slug: %q", got)
	}
}
