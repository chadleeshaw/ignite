package handlers

// import (
// 	"fmt"
// 	"html/template"
// 	"log"
// 	"net"
// 	"net/http"
// 	"strings"
// 	"unicode"

// 	"ignite/config"
// )

// // TFTPDir holds the directory path for TFTP server operations.
// var TFTPDir = config.Defaults.TFTP.Dir

// // HTTPDir holds the directory path for HTTP server operations.
// var HTTPDir = config.Defaults.HTTP.Dir

// // LoadTemplates initializes and returns a map of templates for different pages.
// func LoadTemplates() map[string]*template.Template {
// 	const baseTemplate = "templates/base.templ"
// 	return map[string]*template.Template{
// 		"index":           template.Must(template.ParseFiles(baseTemplate, "templates/pages/index.templ")),
// 		"dhcp":            template.Must(template.ParseFiles(baseTemplate, "templates/pages/dhcp.templ")),
// 		"tftp":            template.Must(template.ParseFiles(baseTemplate, "templates/pages/tftp.templ")),
// 		"status":          template.Must(template.ParseFiles(baseTemplate, "templates/pages/status.templ")),
// 		"provision":       template.Must(template.ParseFiles(baseTemplate, "templates/pages/provision.templ")),
// 		"dhcpmodal":       template.Must(template.ParseFiles("templates/modals/dhcpmodal.templ")),
// 		"reservemodal":    template.Must(template.ParseFiles("templates/modals/reservemodal.templ")),
// 		"bootmodal":       template.Must(template.ParseFiles("templates/modals/bootmodal.templ")),
// 		"ipmimodal":       template.Must(template.ParseFiles("templates/modals/ipmimodal.templ")),
// 		"uploadmodal":     template.Must(template.ParseFiles("templates/modals/uploadmodal.templ")),
// 		"provtempmodal":   template.Must(template.ParseFiles("templates/modals/provtempmodal.templ")),
// 		"provconfigmodal": template.Must(template.ParseFiles("templates/modals/provconfigmodal.templ")),
// 		"provsaveasmodal": template.Must(template.ParseFiles("templates/modals/provsaveasmodal.templ")),
// 	}
// }

// // GetQueryParam retrieves a specific query parameter from the HTTP request.
// func GetQueryParam(r *http.Request, param string) (string, error) {
// 	value := r.URL.Query().Get(param)
// 	if value == "" {
// 		return "", fmt.Errorf("missing %s parameter", param)
// 	}
// 	return value, nil
// }

// // TrimDirectoryPath removes any leading non-letter characters from a path string.
// func TrimDirectoryPath(path string) string {
// 	firstLetterIndex := strings.IndexFunc(path, unicode.IsLetter)
// 	if firstLetterIndex == -1 {
// 		fmt.Println("No letters found in the string.")
// 		return path
// 	}
// 	return path[firstLetterIndex:]
// }

// // setNoCacheHeaders sets HTTP headers to prevent caching.
// func SetNoCacheHeaders(w http.ResponseWriter) {
// 	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
// 	w.Header().Set("Pragma", "no-cache")
// 	w.Header().Set("Expires", "0")
// }

// // Close modal by returning a empty div
// func CloseModalHandler(w http.ResponseWriter, r *http.Request) {
// 	w.Header().Set("Content-Type", "text/html")
// 	w.Write([]byte("<div id=\"modal-content\"></div>"))
// }

// // Open a mdoal given a template query parameter
// func OpenModalHandler(w http.ResponseWriter, r *http.Request) {
// 	template, err := GetQueryParam(r, "template")
// 	if err != nil {
// 		log.Printf("Error retrieving template parameter: %v", err)
// 		http.Error(w, "Invalid template parameter", http.StatusBadRequest)
// 		return
// 	}

// 	templates := LoadTemplates()
// 	if t, ok := templates[template]; !ok {
// 		log.Printf("Template %s not found", template)
// 		http.Error(w, fmt.Sprintf("Template %s not found", template), http.StatusNotFound)
// 		return
// 	} else {
// 		var data map[string]any
// 		var err error

// 		switch template {
// 		case "dhcpmodal":
// 			data = NewDHCPModal()
// 		case "reservemodal":
// 			data, err = NewReserveModal(w, r)
// 			if err != nil {
// 				log.Printf("Error creating reserve modal data: %v", err)
// 				http.Error(w, "Failed to prepare modal data: "+err.Error(), http.StatusInternalServerError)
// 				return
// 			}
// 		case "bootmodal":
// 			data, err = NewBootModal(w, r)
// 			if err != nil {
// 				log.Printf("Error creating boot modal data: %v", err)
// 				http.Error(w, "Failed to prepare boot data: "+err.Error(), http.StatusInternalServerError)
// 				return
// 			}
// 		case "ipmimodal":
// 			data, err = NewIPMIModal(w, r)
// 			if err != nil {
// 				log.Printf("Error creating ipmi modal data: %v", err)
// 				http.Error(w, "Failed to prepare ipmi data: "+err.Error(), http.StatusInternalServerError)
// 				return
// 			}
// 		case "upload":
// 			data = NewUploadModal(w, r)
// 		case "provtempmodal":
// 		case "provconfigmodal":
// 		case "provsaveasmodal":
// 		default:
// 			log.Printf("Unhandled template type: %s", template)
// 			http.Error(w, "Unhandled template type", http.StatusInternalServerError)
// 			return
// 		}

// 		w.Header().Set("Content-Type", "text/html")
// 		if err := t.Execute(w, data); err != nil {
// 			log.Printf("Error executing template %s: %v", template, err)
// 			http.Error(w, "Could not render template", http.StatusInternalServerError)
// 		}
// 	}
// }

// // Check if variable has a default value
// func CheckEmpty(v interface{}) string {
// 	switch val := v.(type) {
// 	case string:
// 		if val == "" {
// 			return ""
// 		}
// 		return val
// 	case bool:
// 		if !val {
// 			return ""
// 		}
// 		return "true"
// 	case net.IP:
// 		if val == nil || val.IsUnspecified() {
// 			return ""
// 		}
// 		return val.String()
// 	default:
// 		return ""
// 	}
// }
