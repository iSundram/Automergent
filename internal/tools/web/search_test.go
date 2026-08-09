package web

import "testing"

func TestSummarizeDuckDuckGoHTML(t *testing.T) {
	html := `<html><body>
<a class="result__a" href="https://example.com/a">Alpha &amp; Beta</a>
<a class="result__a" href="https://example.com/b">Bravo</a>
</body></html>`
	out := summarizeDuckDuckGoHTML("test", html)
	if out == "" {
		t.Fatalf("expected output")
	}
	if out == "no parsed results for query \"test\"" {
		t.Fatalf("expected parsed results")
	}
}
