package handlers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorBundleIsProvisionOnly(t *testing.T) {
	chdirRepoRoot(t)
	const marker = "/public/http/js/editor.js"
	const style = "/public/http/css/editor.css"
	found := false
	err := filepath.Walk("templates", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".templ") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		loads := strings.Contains(text, marker) || strings.Contains(text, style)
		if filepath.Base(path) == "provision.templ" {
			if !loads {
				t.Fatal("provision page does not load the editor bundle")
			}
			found = true
			return nil
		}
		if loads {
			t.Errorf("%s loads the provision editor bundle", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("did not find the provision template")
	}

	prov := renderNamed(t, "provision", &ProvisionData{
		Title: "Provision", Page: "provision", CSRFToken: "session-token", Files: nil,
	})
	if strings.Contains(prov, "previewFile(") || strings.Contains(prov, ">Preview<") {
		t.Fatal("preview control is still on the provision page")
	}
	overview := renderNamed(t, "index", OverviewPage{Title: "Overview", Page: "overview"})
	if strings.Contains(overview, marker) || strings.Contains(overview, style) {
		t.Fatal("overview rendered the provision editor bundle")
	}
}

func TestEditorBundleStaysText(t *testing.T) {
	chdirRepoRoot(t)
	cmd := exec.Command("node", "scripts/editor_test.mjs")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("editor test: %v\n%s", err, out)
	}
}
