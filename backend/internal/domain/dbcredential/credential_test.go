package dbcredential

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// The UI never sends a username, and NewCredential used to reject an empty one,
// so every "create credential" click returned 422 and the service's
// auto-generate branch was unreachable.
func TestNewCredentialGeneratesUsername(t *testing.T) {
	cred, err := NewCredential(uuid.New(), "My App!", "", AccessReadonly, "")
	if err != nil {
		t.Fatalf("NewCredential: %v", err)
	}
	if cred.Username == "" {
		t.Fatal("username must be generated when none is supplied")
	}
	if !strings.HasPrefix(cred.Username, "my_app_") {
		t.Errorf("username %q should be derived from the name", cred.Username)
	}
	if !usernamePattern.MatchString(cred.Username) {
		t.Errorf("generated username %q is not a valid SQL identifier", cred.Username)
	}
}

func TestNewCredentialUsernamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		cred, err := NewCredential(uuid.New(), "app", "", AccessReadonly, "")
		if err != nil {
			t.Fatalf("NewCredential: %v", err)
		}
		if seen[cred.Username] {
			t.Fatalf("duplicate generated username %q", cred.Username)
		}
		seen[cred.Username] = true
	}
}

func TestNewCredentialHonoursExplicitUsername(t *testing.T) {
	cred, err := NewCredential(uuid.New(), "App", "app_reader", AccessReadonly, "")
	if err != nil {
		t.Fatalf("NewCredential: %v", err)
	}
	if cred.Username != "app_reader" {
		t.Fatalf("username = %q, want app_reader", cred.Username)
	}
}

// A name that slugifies to nothing must still yield a usable identifier rather
// than a bare "_1a2b3c".
func TestGenerateUsernameFallsBackForUnusableNames(t *testing.T) {
	for _, name := range []string{"!!!", "上海", "  ", "123"} {
		got, err := GenerateUsername(name)
		if err != nil {
			t.Fatalf("GenerateUsername(%q): %v", name, err)
		}
		if !usernamePattern.MatchString(got) {
			t.Errorf("GenerateUsername(%q) = %q, not a valid identifier", name, got)
		}
	}
}

func TestGenerateUsernameRespectsLengthLimit(t *testing.T) {
	got, err := GenerateUsername(strings.Repeat("verylongname", 20))
	if err != nil {
		t.Fatalf("GenerateUsername: %v", err)
	}
	if !usernamePattern.MatchString(got) {
		t.Fatalf("truncated username %q is not a valid identifier (len %d)", got, len(got))
	}
}

func TestNewCredentialRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		credName  string
		username  string
		access    AccessLevel
		wantField string
	}{
		{"empty name", "", "", AccessReadonly, "name"},
		{"name too long", strings.Repeat("a", 65), "", AccessReadonly, "name"},
		// An unquoted identifier with these characters would break the SQL that
		// creates the account.
		{"username with quotes", "App", `bad"name`, AccessReadonly, "username"},
		{"username with spaces", "App", "bad name", AccessReadonly, "username"},
		{"username starting with a digit", "App", "1app", AccessReadonly, "username"},
		{"username too long", "App", strings.Repeat("a", 64), AccessReadonly, "username"},
		{"invalid access level", "App", "app", AccessLevel("superuser"), "access_level"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCredential(uuid.New(), tc.credName, tc.username, tc.access, "")
			var appErr *apperr.Error
			if !errorsAs(err, &appErr) {
				t.Fatalf("expected an apperr, got %v", err)
			}
			if appErr.Field != tc.wantField {
				t.Fatalf("field = %q, want %q", appErr.Field, tc.wantField)
			}
		})
	}
}

func TestNewCredentialDefaultsAccountHost(t *testing.T) {
	cred, err := NewCredential(uuid.New(), "App", "app", AccessReadonly, "")
	if err != nil {
		t.Fatalf("NewCredential: %v", err)
	}
	if cred.AccountHost != "%" {
		t.Fatalf("account host = %q, want %%", cred.AccountHost)
	}
}

func errorsAs(err error, target **apperr.Error) bool {
	e, ok := err.(*apperr.Error)
	if ok {
		*target = e
	}
	return ok
}
