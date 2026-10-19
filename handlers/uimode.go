package handlers

import (
	"net/http"
	"reflect"
	"sync"
)

// preferenceStore is the process-wide store page renders consult so every
// template execution can carry .Mode without each handler threading it through.
var (
	prefMu          sync.RWMutex
	preferenceStore PreferenceStore
)

// SetPreferenceStore installs the store used when rendering pages.
// A nil store makes every page render with DefaultUIMode.
func SetPreferenceStore(store PreferenceStore) {
	prefMu.Lock()
	preferenceStore = store
	prefMu.Unlock()
}

func currentUsername() string {
	credentialMu.RLock()
	defer credentialMu.RUnlock()
	return defaultUsername
}

func currentUIMode(r *http.Request) string {
	if r == nil {
		return DefaultUIMode
	}
	// Login has no session yet. It still reads the stored preference so the
	// page matches the density the user saved. Every other anonymous page
	// stays on the default until they sign in.
	if !isAuthenticated(r) && r.URL.Path != "/login" {
		return DefaultUIMode
	}
	prefMu.RLock()
	store := preferenceStore
	prefMu.RUnlock()
	if store == nil {
		return DefaultUIMode
	}
	mode, err := store.GetUIMode(r.Context(), currentUsername())
	if err != nil || !ValidUIMode(mode) {
		return DefaultUIMode
	}
	return mode
}

// applyUIMode returns data with Mode set to the signed-in user's density
// and CSRFToken set to that session's token.
// A request with no session (except GET /login), a failed lookup, or an
// unknown stored value gets DefaultUIMode. Maps gain "Mode" and "CSRFToken"
// keys. Structs with exported Mode or CSRFToken string fields are updated
// (a non-pointer struct is copied first). Nil data becomes a small struct
// carrying Title, Mode, and CSRFToken so base templates can render. The HTML
// data-theme attribute stays "ignite" until the per-mode themes exist; Mode
// is still passed through.
func applyUIMode(r *http.Request, data any) any {
	mode := currentUIMode(r)
	csrf := currentCSRFToken(r)
	if data == nil {
		return struct {
			Title     string
			Mode      string
			CSRFToken string
		}{Mode: mode, CSRFToken: csrf}
	}

	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return struct {
				Title     string
				Mode      string
				CSRFToken string
			}{Mode: mode, CSRFToken: csrf}
		}
		if v.Elem().Kind() == reflect.Struct {
			setPageFields(v.Elem(), mode, csrf)
		}
		return data
	case reflect.Struct:
		cp := reflect.New(v.Type())
		cp.Elem().Set(v)
		setPageFields(cp.Elem(), mode, csrf)
		return cp.Interface()
	case reflect.Map:
		setMapPageFields(v, mode, csrf)
		return data
	default:
		return data
	}
}

func setPageFields(v reflect.Value, mode, csrf string) {
	setStringField(v, "Mode", mode)
	setStringField(v, "CSRFToken", csrf)
}

func setStringField(v reflect.Value, name, value string) {
	if !v.IsValid() || !v.CanSet() {
		return
	}
	field := v.FieldByName(name)
	if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
		field.SetString(value)
	}
}

func setMapPageFields(v reflect.Value, mode, csrf string) {
	setMapString(v, "Mode", mode)
	setMapString(v, "CSRFToken", csrf)
}

func setMapString(v reflect.Value, key, value string) {
	if !v.IsValid() || v.IsNil() || v.Type().Key().Kind() != reflect.String {
		return
	}
	rv := reflect.ValueOf(value)
	if !rv.Type().AssignableTo(v.Type().Elem()) {
		return
	}
	v.SetMapIndex(reflect.ValueOf(key), rv)
}
