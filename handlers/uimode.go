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

// pageHealthFn supplies OverallStatus for every HTML render. Nil leaves
// the field empty, which tests use when they are not checking health.
var (
	pageHealthMu sync.RWMutex
	pageHealthFn func(*http.Request) string
)

// SetPageHealth installs the function that fills OverallStatus on every
// page render. The application wires it to the same overall service word
// as /status/content. Pass nil to clear it.
func SetPageHealth(fn func(*http.Request) string) {
	pageHealthMu.Lock()
	pageHealthFn = fn
	pageHealthMu.Unlock()
}

func pageHealth(r *http.Request) string {
	pageHealthMu.RLock()
	fn := pageHealthFn
	pageHealthMu.RUnlock()
	if fn == nil || r == nil {
		return ""
	}
	return fn(r)
}

// applyUIMode returns data with Mode set to the signed-in user's density,
// CSRFToken set to that session's token, and OverallStatus set from
// SetPageHealth when that value is non-empty. An empty value leaves a
// status the handler already stored. A request with no session (except GET /login), a failed
// lookup, or an unknown stored value gets DefaultUIMode. Maps gain "Mode",
// "CSRFToken", and "OverallStatus" keys. Structs with those exported string
// fields are updated (a non-pointer struct is copied first). Nil data
// becomes a small struct carrying Title, Mode, CSRFToken, and OverallStatus
// so base templates can render. The HTML data-theme attribute stays
// "ignite" until the per-mode themes exist; Mode is still passed through.
func applyUIMode(r *http.Request, data any) any {
	mode := currentUIMode(r)
	csrf := currentCSRFToken(r)
	health := pageHealth(r)
	if data == nil {
		return struct {
			Title         string
			Mode          string
			CSRFToken     string
			OverallStatus string
		}{Mode: mode, CSRFToken: csrf, OverallStatus: health}
	}

	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return struct {
				Title         string
				Mode          string
				CSRFToken     string
				OverallStatus string
			}{Mode: mode, CSRFToken: csrf, OverallStatus: health}
		}
		if v.Elem().Kind() == reflect.Struct {
			setPageFields(v.Elem(), mode, csrf, health)
		}
		return data
	case reflect.Struct:
		cp := reflect.New(v.Type())
		cp.Elem().Set(v)
		setPageFields(cp.Elem(), mode, csrf, health)
		return cp.Interface()
	case reflect.Map:
		setMapPageFields(v, mode, csrf, health)
		return data
	default:
		return data
	}
}

func setPageFields(v reflect.Value, mode, csrf, health string) {
	setStringField(v, "Mode", mode)
	setStringField(v, "CSRFToken", csrf)
	// An empty callback must not wipe a status the handler already computed.
	// /status/content sets OverallStatus itself. Other pages leave it empty
	// and receive the shared value when one is available.
	if health != "" {
		setStringField(v, "OverallStatus", health)
	}
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

func setMapPageFields(v reflect.Value, mode, csrf, health string) {
	setMapString(v, "Mode", mode)
	setMapString(v, "CSRFToken", csrf)
	if health != "" {
		setMapString(v, "OverallStatus", health)
	}
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
