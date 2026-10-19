package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// ErrorType represents different types of errors for better categorization
type ErrorType string

const (
	ErrorTypeValidation     ErrorType = "validation"
	ErrorTypeNotFound       ErrorType = "not_found"
	ErrorTypeUnauthorized   ErrorType = "unauthorized"
	ErrorTypeInternal       ErrorType = "internal"
	ErrorTypeConflict       ErrorType = "conflict"
	ErrorTypeForbidden      ErrorType = "forbidden"
	ErrorTypeRateLimit      ErrorType = "rate_limit"
	ErrorTypeServiceUnavail ErrorType = "service_unavailable"
)

// AppError represents a structured application error
type AppError struct {
	Type        ErrorType   `json:"type"`
	Message     string      `json:"message"`
	UserMessage string      `json:"user_message"`
	Code        string      `json:"code,omitempty"`
	Details     interface{} `json:"details,omitempty"`
	StatusCode  int         `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	return e.Message
}

// NewAppError creates a new AppError
func NewAppError(errorType ErrorType, message, userMessage string, statusCode int) *AppError {
	return &AppError{
		Type:        errorType,
		Message:     message,
		UserMessage: userMessage,
		StatusCode:  statusCode,
	}
}

// WithCode adds an error code to the AppError
func (e *AppError) WithCode(code string) *AppError {
	e.Code = code
	return e
}

// WithDetails adds details to the AppError
func (e *AppError) WithDetails(details interface{}) *AppError {
	e.Details = details
	return e
}

// Common error constructors
func NewValidationError(message, userMessage string) *AppError {
	return NewAppError(ErrorTypeValidation, message, userMessage, http.StatusBadRequest)
}

func NewNotFoundError(message, userMessage string) *AppError {
	return NewAppError(ErrorTypeNotFound, message, userMessage, http.StatusNotFound)
}

func NewInternalError(message, userMessage string) *AppError {
	return NewAppError(ErrorTypeInternal, message, userMessage, http.StatusInternalServerError)
}

// ErrorResponse represents the structure of error responses
type ErrorResponse struct {
	Success bool     `json:"success"`
	Error   AppError `json:"error"`
}

// SendError sends a structured error response.
// The internal Message is logged server-side only; the response carries the
// generic UserMessage so internal details never leak to clients.
func SendError(w http.ResponseWriter, r *http.Request, err *AppError) {
	// Log the internal error message
	log.Printf("Error [%s] %s: %s", r.Method, r.URL.Path, err.Message)
	if err.Details != nil {
		log.Printf("Error details: %+v", err.Details)
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)

	// Create error response — expose only the user-facing message
	response := ErrorResponse{
		Success: false,
		Error: AppError{
			Type:        err.Type,
			Message:     err.UserMessage,
			UserMessage: err.UserMessage,
			Code:        err.Code,
			StatusCode:  err.StatusCode,
		},
	}

	// Send JSON response
	if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
		log.Printf("Failed to encode error response: %v", encodeErr)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// errorPageData is the view model for HTML error pages.
type errorPageData struct {
	Title      string
	Mode       string
	CSRFToken  string
	StatusCode int
	Heading    string
	Message    string
}

// SendHTMLError renders the shared error page. Mode comes from the signed-in
// user's preference, or cozy when there is no session. A failure to render
// falls back to a plain response and does not call back into HandleError.
func SendHTMLError(w http.ResponseWriter, r *http.Request, err *AppError) {
	log.Printf("HTML Error [%s] %s: %s", r.Method, r.URL.Path, err.Message)
	if err.Details != nil {
		log.Printf("Error details: %+v", err.Details)
	}

	code := err.StatusCode
	if code == 0 {
		code = http.StatusInternalServerError
	}
	heading := http.StatusText(code)
	if heading == "" {
		heading = "Error"
	}
	message := err.UserMessage
	if message == "" {
		message = heading
	}

	page := errorPageData{
		Title:      heading,
		StatusCode: code,
		Heading:    heading,
		Message:    message,
	}
	// Unauthenticated 404/405 handlers run outside AuthMiddleware. They must
	// not render the admin shell (nav links to /dhcp and the rest).
	name := "error"
	if !isAuthenticated(r) {
		name = "error-public"
	}
	tmpl := LoadTemplates()[name]
	if tmpl == nil {
		http.Error(w, message, code)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if execErr := tmpl.Execute(w, applyUIMode(r, page)); execErr != nil {
		log.Printf("failed to render error page: %v", execErr)
	}
}

// NotFound renders the HTML 404 page for routes that did not match.
func NotFound(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, NewNotFoundError(
		"no route matched "+r.URL.Path,
		"The page you requested was not found.",
	))
}

// MethodNotAllowed renders the HTML error page when the path exists but the
// method does not.
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, NewAppError(
		ErrorTypeValidation,
		r.Method+" is not allowed for "+r.URL.Path,
		"That action is not available on this page.",
		http.StatusMethodNotAllowed,
	))
}

// WrapError wraps a standard error into an AppError
func WrapError(err error, errorType ErrorType, userMessage string, statusCode int) *AppError {
	if err == nil {
		return nil
	}

	if appErr, ok := err.(*AppError); ok {
		return appErr
	}

	return NewAppError(errorType, err.Error(), userMessage, statusCode)
}

// ValidationErrors represents a collection of validation errors
type ValidationErrors map[string][]string

// Add adds a validation error for a field
func (ve ValidationErrors) Add(field, message string) {
	ve[field] = append(ve[field], message)
}

// HasErrors returns true if there are any validation errors
func (ve ValidationErrors) HasErrors() bool {
	return len(ve) > 0
}

// ToAppError converts ValidationErrors to an AppError
func (ve ValidationErrors) ToAppError() *AppError {
	if !ve.HasErrors() {
		return nil
	}

	return NewValidationError(
		"Validation failed",
		"Please check your input and try again",
	).WithDetails(ve)
}

// SendValidationError sends validation errors
func SendValidationError(w http.ResponseWriter, r *http.Request, errors ValidationErrors) {
	if appErr := errors.ToAppError(); appErr != nil {
		SendError(w, r, appErr)
	}
}

// IsHTMLRequest checks if the request expects HTML response.
// A wildcard Accept of */* (curl/fetch default) does NOT count as HTML —
// only an explicit text/html or application/xhtml+xml does.
func IsHTMLRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return true // Default to HTML for browser-like requests
	}
	return contains(accept, "text/html") ||
		contains(accept, "application/xhtml+xml")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// HandleError is a utility function that automatically chooses between JSON and HTML error responses.
// API paths and explicit application/json clients always get JSON; HTML only
// when the Accept header actually asks for it.
func HandleError(w http.ResponseWriter, r *http.Request, err *AppError) {
	if wantsJSONResponse(r) {
		SendError(w, r, err)
		return
	}
	if IsHTMLRequest(r) && r.Header.Get("HX-Request") == "" { // Not an HTMX request
		SendHTMLError(w, r, err)
	} else {
		SendError(w, r, err)
	}
}

// wantsJSONResponse reports whether the request should get a JSON error body.
func wantsJSONResponse(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	return contains(r.Header.Get("Accept"), "application/json")
}

// Common error messages
var (
	ErrInvalidInput     = "Invalid input provided"
	ErrResourceNotFound = "The requested resource was not found"
	ErrUnauthorized     = "You are not authorized to perform this action"
	ErrInternalServer   = "An internal server error occurred"
	ErrServiceUnavail   = "The service is temporarily unavailable"
	ErrConflict         = "The request conflicts with the current state"
	ErrForbidden        = "Access to this resource is forbidden"
)
