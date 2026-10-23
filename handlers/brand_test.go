package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServedBrandMarkStaysSmall(t *testing.T) {
	chdirRepoRoot(t)
	entries, err := os.ReadDir("public/http/img")
	if err != nil {
		t.Fatal(err)
	}
	var brand int
	for _, entry := range entries {
		name := entry.Name()
		if name == "Ignite_small.png" || name == "flame.svg" {
			t.Fatalf("public image %s should not be served", name)
		}
		if !strings.HasPrefix(name, "flame") && !strings.HasPrefix(name, "favicon") && !strings.HasPrefix(name, "apple-touch") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() >= 30*1024 {
			t.Fatalf("%s is %d bytes", name, info.Size())
		}
		brand++
	}
	if brand < 6 {
		t.Fatalf("expected the nav, favicon, and touch assets, found %d", brand)
	}
	source := filepath.Join("assets", "brand", "Ignite_small.png")
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 100*1024 {
		t.Fatalf("brand source looks truncated: %d", info.Size())
	}
	pages, err := os.ReadFile("templates/base.templ")
	if err != nil {
		t.Fatal(err)
	}
	body := string(pages)
	for _, want := range []string{"flame-nav.png", "favicon-32.png", "apple-touch-icon.png"} {
		if !strings.Contains(body, want) {
			t.Fatalf("base template missing %s", want)
		}
	}
	if strings.Contains(body, "Ignite_small.png") || strings.Contains(body, "flame.svg") {
		t.Fatal("base template still points at the old mark")
	}
}
