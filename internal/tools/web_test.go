package tools

import (
	"strings"
	"testing"
)

func TestUrlEncodeASCII(t *testing.T) {
	input := "hello-world_123.txt~backup"
	got := urlEncode(input)
	if got != input {
		t.Fatalf("ASCII passthrough failed: expected %q, got %q", input, got)
	}
}

func TestUrlEncodeSpaces(t *testing.T) {
	got := urlEncode("hello world")
	expected := "hello%20world"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestUrlEncodeChinese(t *testing.T) {
	got := urlEncode("中文")
	expected := "%E4%B8%AD%E6%96%87"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestUrlEncodeMixed(t *testing.T) {
	got := urlEncode("hello 世界")
	expected := "hello%20%E4%B8%96%E7%95%8C"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestUrlEncodeSpecialChars(t *testing.T) {
	// Characters like !, *, (, ) should be percent-encoded (they are not in the unreserved set)
	got := urlEncode("a=b&c=d")
	if strings.Contains(got, "=") {
		t.Fatalf("expected '=' to be encoded, got %q", got)
	}
	if strings.Contains(got, "&") {
		t.Fatalf("expected '&' to be encoded, got %q", got)
	}
}

func TestUrlDecodeBasic(t *testing.T) {
	input := "hello%20world"
	got := urlDecode(input)
	expected := "hello world"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestUrlDecodeChinese(t *testing.T) {
	input := "%E4%B8%AD%E6%96%87"
	got := urlDecode(input)
	expected := "中文"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestUrlDecodePlus(t *testing.T) {
	got := urlDecode("hello+world")
	expected := "hello world"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestUrlEncodeDecodeRoundTrip(t *testing.T) {
	cases := []string{
		"hello world",
		"中文测试",
		"mixed ASCII and 日本語",
		"special chars: !@#$^",
		"path/to/file.go",
	}
	for _, original := range cases {
		encoded := urlEncode(original)
		decoded := urlDecode(encoded)
		if decoded != original {
			t.Errorf("round-trip failed for %q: encoded=%q, decoded=%q", original, encoded, decoded)
		}
	}
}

func TestStripHTMLRemovesTags(t *testing.T) {
	input := "<p>Hello <b>World</b></p>"
	got := stripHTML(input)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Fatalf("expected all HTML tags removed, got %q", got)
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Fatalf("expected text content preserved, got %q", got)
	}
}

func TestStripHTMLRemovesScriptContent(t *testing.T) {
	input := `<p>Before</p><script>alert("xss")</script><p>After</p>`
	got := stripHTML(input)
	if strings.Contains(got, "alert") || strings.Contains(got, "xss") {
		t.Fatalf("expected script content removed, got %q", got)
	}
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Fatalf("expected surrounding text preserved, got %q", got)
	}
}

func TestStripHTMLRemovesStyleContent(t *testing.T) {
	input := `<style>body { color: red; }</style><p>Content</p>`
	got := stripHTML(input)
	if strings.Contains(got, "color") || strings.Contains(got, "red") {
		t.Fatalf("expected style content removed, got %q", got)
	}
	if !strings.Contains(got, "Content") {
		t.Fatalf("expected text content preserved, got %q", got)
	}
}

func TestStripHTMLPreservesTextContent(t *testing.T) {
	input := "Just plain text with no HTML"
	got := stripHTML(input)
	if got != "Just plain text with no HTML" {
		t.Fatalf("expected plain text unchanged, got %q", got)
	}
}

func TestStripHTMLEmptyInput(t *testing.T) {
	got := stripHTML("")
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestStripHTMLNestedTags(t *testing.T) {
	input := `<div><ul><li>Item 1</li><li>Item 2</li></ul></div>`
	got := stripHTML(input)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Fatalf("expected all tags removed, got %q", got)
	}
	if !strings.Contains(got, "Item 1") || !strings.Contains(got, "Item 2") {
		t.Fatalf("expected list items preserved, got %q", got)
	}
}
