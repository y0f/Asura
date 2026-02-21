package api

import (
	"strings"
	"testing"
)

func TestSanitizeCSSAllowsValidProperties(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic color",
			input: `.card { color: red; }`,
			want:  `.card { color: red }`,
		},
		{
			name:  "multiple properties",
			input: `.card { color: #fff; background-color: #1a1a1a; border-radius: 8px; }`,
			want:  `.card { color: #fff; background-color: #1a1a1a; border-radius: 8px }`,
		},
		{
			name:  "spacing properties",
			input: `.box { margin: 10px; padding: 20px 15px; }`,
			want:  `.box { margin: 10px; padding: 20px 15px }`,
		},
		{
			name:  "font properties",
			input: `body { font-size: 16px; font-weight: bold; font-family: Arial, sans-serif; }`,
			want:  `body { font-size: 16px; font-weight: bold; font-family: Arial, sans-serif }`,
		},
		{
			name:  "flex layout",
			input: `.flex { display: flex; flex-direction: column; gap: 1rem; }`,
			want:  `.flex { display: flex; flex-direction: column; gap: 1rem }`,
		},
		{
			name:  "box shadow",
			input: `.shadow { box-shadow: 0 2px 4px rgba(0,0,0,0.2); }`,
			want:  `.shadow { box-shadow: 0 2px 4px rgba(0,0,0,0.2) }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeCSS(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeCSS() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeCSSBlocksDangerous(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "javascript url",
			input: `.x { background: url(javascript:alert(1)); }`,
		},
		{
			name:  "expression",
			input: `.x { width: expression(document.body.clientWidth); }`,
		},
		{
			name:  "import rule",
			input: `@import url("evil.css"); .x { color: red; }`,
		},
		{
			name:  "behavior",
			input: `.x { behavior: url(xss.htc); }`,
		},
		{
			name:  "moz-binding",
			input: `.x { -moz-binding: url("xss.xml#xss"); }`,
		},
		{
			name:  "data uri in value",
			input: `.x { background: data:text/html,<script>alert(1)</script>; }`,
		},
		{
			name:  "vbscript",
			input: `.x { background: vbscript:msgbox; }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeCSS(tt.input)
			lower := strings.ToLower(got)
			for _, dangerous := range []string{"javascript", "expression", "@import", "behavior", "-moz-binding", "data:", "vbscript"} {
				if strings.Contains(lower, dangerous) {
					t.Errorf("sanitizeCSS() should not contain %q, got %q", dangerous, got)
				}
			}
		})
	}
}

func TestSanitizeCSSBlocksUnknownProperties(t *testing.T) {
	input := `.x { -webkit-animation: evil; unknown-prop: value; color: red; }`
	got := sanitizeCSS(input)

	if strings.Contains(got, "-webkit-animation") {
		t.Error("should strip -webkit-animation")
	}
	if strings.Contains(got, "unknown-prop") {
		t.Error("should strip unknown-prop")
	}
	if !strings.Contains(got, "color: red") {
		t.Error("should keep color: red")
	}
}

func TestSanitizeCSSStripsComments(t *testing.T) {
	input := `.x { color: red; /* comment */ font-size: 14px; }`
	got := sanitizeCSS(input)

	if strings.Contains(got, "comment") {
		t.Error("should strip CSS comments")
	}
	if !strings.Contains(got, "color: red") {
		t.Error("should keep color: red")
	}
	if !strings.Contains(got, "font-size: 14px") {
		t.Error("should keep font-size: 14px")
	}
}

func TestSanitizeCSSStripsHTMLTags(t *testing.T) {
	input := `.x { color: red; } <script>alert(1)</script>`
	got := sanitizeCSS(input)

	if strings.Contains(got, "script") {
		t.Error("should strip HTML tags")
	}
}

func TestSanitizeCSSMultipleRules(t *testing.T) {
	input := `.a { color: red; } .b { font-size: 14px; }`
	got := sanitizeCSS(input)

	if !strings.Contains(got, ".a") || !strings.Contains(got, ".b") {
		t.Errorf("should preserve multiple rules, got %q", got)
	}
}

func TestSanitizeCSSMaxLength(t *testing.T) {
	input := strings.Repeat("x", 20000)
	got := sanitizeCSS(input)
	if len(got) > 10000 {
		t.Error("should truncate at 10000 chars")
	}
}

func TestSanitizeCSSEmpty(t *testing.T) {
	if got := sanitizeCSS(""); got != "" {
		t.Errorf("empty input should give empty output, got %q", got)
	}
}

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-page", "my-page"},
		{"test", "test"},
		{"", "status"},
		{"LOGIN", "status"},
		{"login", "status"},
		{"a", "a"},
		{"ab", "ab"},
		{"a-b", "a-b"},
		{"has spaces", "status"},
		{"a--b", "a--b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := validateSlug(tt.input); got != tt.want {
				t.Errorf("validateSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
