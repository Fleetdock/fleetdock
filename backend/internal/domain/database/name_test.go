package database

import "testing"

func TestValidateName(t *testing.T) {
	for _, name := range []string{"test", "acceptix_prod", "tenant_ffc07774-1db5-47fc-8bf5-ef388bcba207", "a$b"} {
		if err := validateName(name); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{"", "-x", "a b", "a.b", "a`b"} {
		if err := validateName(name); err == nil {
			t.Errorf("validateName(%q) = nil, want an error", name)
		}
	}
}
