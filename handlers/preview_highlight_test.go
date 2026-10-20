package handlers

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestHighlightScriptStaysText(t *testing.T) {
	chdirRepoRoot(t)
	body, err := os.ReadFile("public/http/js/highlight.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML"} {
		if strings.Contains(src, sink) {
			t.Fatalf("highlighter contains %s", sink)
		}
	}
	if !strings.Contains(src, "createElement") || !strings.Contains(src, "textContent") {
		t.Fatal("highlighter does not build DOM nodes")
	}
	cmd := exec.Command("node", "scripts/highlight_test.mjs")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("highlight test: %v\n%s", err, out)
	}
}
