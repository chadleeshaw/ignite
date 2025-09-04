package handlers

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"unicode"
)

// LoadTemplates initializes and returns a map of templates for different pages.
func LoadTemplates() map[string]*template.Template {
	const baseTemplate = "templates/base.templ"
	return map[string]*template.Template{
		"index":           template.Must(template.ParseFiles(baseTemplate, "templates/pages/index.templ")),
		"dhcp":            template.Must(template.ParseFiles(baseTemplate, "templates/pages/dhcp.templ")),
		"tftp":            template.Must(template.ParseFiles(baseTemplate, "templates/pages/tftp.templ")),
		"status":          template.Must(template.ParseFiles(baseTemplate, "templates/pages/status.templ")),
		"provision":       template.Must(template.ParseFiles(baseTemplate, "templates/pages/provision.templ")),
		"dhcpmodal":       template.Must(template.ParseFiles("templates/modals/dhcpmodal.templ")),
		"reservemodal":    template.Must(template.ParseFiles("templates/modals/reservemodal.templ")),
		"bootmodal":       template.Must(template.ParseFiles("templates/modals/bootmodal.templ")),
		"ipmimodal":       template.Must(template.ParseFiles("templates/modals/ipmimodal.templ")),
		"uploadmodal":     template.Must(template.ParseFiles("templates/modals/uploadmodal.templ")),
		"provtempmodal":   template.Must(template.ParseFiles("templates/modals/provtempmodal.templ")),
		"provconfigmodal": template.Must(template.ParseFiles("templates/modals/provconfigmodal.templ")),
		"provsaveasmodal": template.Must(template.ParseFiles("templates/modals/provsaveasmodal.templ")),
	}
}

// TrimDirectoryPath removes any leading non-letter characters from a path string.
func TrimDirectoryPath(path string) string {
	firstLetterIndex := strings.IndexFunc(path, unicode.IsLetter)
	if firstLetterIndex == -1 {
		fmt.Println("No letters found in the string.")
		return path
	}
	return path[firstLetterIndex:]
}

// SetNoCacheHeaders sets HTTP headers to prevent caching.
func SetNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// CheckEmpty checks if a variable has a default value and returns its string representation.
func CheckEmpty(v interface{}) string {
	switch val := v.(type) {
	case string:
		if val == "" {
			return ""
		}
		return val
	case bool:
		if !val {
			return ""
		}
		return "true"
	case net.IP:
		if val == nil || val.IsUnspecified() {
			return ""
		}
		return val.String()
	default:
		return ""
	}
}
