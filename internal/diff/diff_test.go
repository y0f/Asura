package diff

import (
	"strings"
	"testing"
)

func TestComputeIdentical(t *testing.T) {
	result := Compute("hello\nworld", "hello\nworld")
	if strings.Contains(result, "+") || strings.Contains(result, "-") {
		t.Fatalf("expected no changes, got:\n%s", result)
	}
}

func TestComputeAddition(t *testing.T) {
	result := Compute("line1\nline2", "line1\nline2\nline3")
	if !strings.Contains(result, "+line3") {
		t.Fatalf("expected addition of line3, got:\n%s", result)
	}
}

func TestComputeDeletion(t *testing.T) {
	result := Compute("line1\nline2\nline3", "line1\nline3")
	if !strings.Contains(result, "-line2") {
		t.Fatalf("expected deletion of line2, got:\n%s", result)
	}
}

func TestComputeModification(t *testing.T) {
	result := Compute("hello\nworld", "hello\nearth")
	if !strings.Contains(result, "-world") || !strings.Contains(result, "+earth") {
		t.Fatalf("expected modification, got:\n%s", result)
	}
}

func TestComputeEmpty(t *testing.T) {
	result := Compute("", "new content")
	if !strings.Contains(result, "+new content") {
		t.Fatalf("expected addition, got:\n%s", result)
	}

	result = Compute("old content", "")
	if !strings.Contains(result, "-old content") {
		t.Fatalf("expected deletion, got:\n%s", result)
	}
}

func TestComputeBothEmpty(t *testing.T) {
	result := Compute("", "")
	if result != "" {
		t.Fatalf("expected empty diff, got:\n%s", result)
	}
}
