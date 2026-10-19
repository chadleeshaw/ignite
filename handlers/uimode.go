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

// applyUIMode returns data with Mode set to the signed-in user's density.
// A request with no session (except GET /login), a failed lookup, or an
// unknown stored value gets DefaultUIMode. Maps gain a "Mode" key. Structs
// with an exported Mode string field are updated (a non-pointer struct is
// copied first). Nil data becomes a small struct carrying Title and Mode
// so base templates can render. The HTML data-theme attribute stays
// "ignite" until the per-mode themes exist; Mode is still passed through.
func applyUIMode(r *http.Request, data any) any {
	mode := currentUIMode(r)
	if data == nil {
		return struct {
			Title string
			Mode  string
		}{Mode: mode}
	}

	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return struct {
				Title string
				Mode  string
			}{Mode: mode}
		}
		if v.Elem().Kind() == reflect.Struct {
			setStructMode(v.Elem(), mode)
		}
		return data
	case reflect.Struct:
		cp := reflect.New(v.Type())
		cp.Elem().Set(v)
		setStructMode(cp.Elem(), mode)
		return cp.Interface()
	case reflect.Map:
		setMapMode(v, mode)
		return data
	default:
		return data
	}
}

func setStructMode(v reflect.Value, mode string) {
	if !v.IsValid() || !v.CanSet() {
		return
	}
	field := v.FieldByName("Mode")
	if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
		field.SetString(mode)
	}
}

func setMapMode(v reflect.Value, mode string) {
	if !v.IsValid() || v.IsNil() || v.Type().Key().Kind() != reflect.String {
		return
	}
	value := reflect.ValueOf(mode)
	if !value.Type().AssignableTo(v.Type().Elem()) {
		return
	}
	v.SetMapIndex(reflect.ValueOf("Mode"), value)
}
