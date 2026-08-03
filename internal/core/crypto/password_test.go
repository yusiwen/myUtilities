package crypto

import (
	"testing"
	"unicode"
)

func TestPasswordDefaultLength(t *testing.T) {
	pw, err := GeneratePassword(DefaultPasswordLength)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(pw)) != DefaultPasswordLength {
		t.Fatalf("expected %d chars, got %d", DefaultPasswordLength, len([]rune(pw)))
	}
}

func TestPasswordCustomLength(t *testing.T) {
	for _, l := range []int{12, 20, 40} {
		pw, err := GeneratePassword(l)
		if err != nil {
			t.Fatalf("length %d: %v", l, err)
		}
		if len([]rune(pw)) != l {
			t.Fatalf("length %d: got %d chars", l, len([]rune(pw)))
		}
	}
}

func TestPasswordMinEnforced(t *testing.T) {
	pw, err := GeneratePassword(3)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(pw)) != MinPasswordLength {
		t.Fatalf("expected min %d chars, got %d", MinPasswordLength, len([]rune(pw)))
	}
}

func TestPasswordLargeLength(t *testing.T) {
	pw, err := GeneratePassword(256)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(pw)) != 256 {
		t.Fatalf("expected 256 chars, got %d", len([]rune(pw)))
	}
}

func TestPasswordRandomness(t *testing.T) {
	a, err := GeneratePassword(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratePassword(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two generated passwords were identical")
	}
}

func TestPasswordCharset(t *testing.T) {
	pw, err := GeneratePassword(100)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range pw {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			t.Fatalf("invalid char in password: %c", r)
		}
	}
}
