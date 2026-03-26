package createdb

import (
	"strings"
	"testing"
)

func TestGeneratePassword_Length(t *testing.T) {
	for _, length := range []int{10, 20, 32} {
		pw, err := GeneratePassword(length)
		if err != nil {
			t.Fatalf("GeneratePassword(%d): %v", length, err)
		}
		if len(pw) != length {
			t.Errorf("GeneratePassword(%d): got length %d", length, len(pw))
		}
	}
}

func TestGeneratePassword_AlphanumericOnly(t *testing.T) {
	pw, err := GeneratePassword(100)
	if err != nil {
		t.Fatalf("GeneratePassword: %v", err)
	}
	for _, c := range pw {
		if !strings.ContainsRune(alphanumeric, c) {
			t.Errorf("GeneratePassword: contains non-alphanumeric char %q", string(c))
		}
	}
}

func TestGeneratePassword_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		pw, err := GeneratePassword(20)
		if err != nil {
			t.Fatalf("GeneratePassword: %v", err)
		}
		if seen[pw] {
			t.Errorf("GeneratePassword: duplicate password on attempt %d", i)
		}
		seen[pw] = true
	}
}
