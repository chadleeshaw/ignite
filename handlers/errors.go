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

// SendError sends a structured error response
func SendError(w http.ResponseWriter, r *http.Request, err *AppError) {
	// Log the internal error message
	log.Printf("Error [%s] %s: %s", r.Method, r.URL.Path, err.Message)
	if err.Details != nil {
		log.Printf("Error details: %+v", err.Details)
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)

	// Create error response
	response := ErrorResponse{
		Success: false,
		Error:   *err,
	}

	// Send JSON response
	if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
		log.Printf("Failed to encode error response: %v", encodeErr)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// SendHTMLError sends an error for HTML responses (for template-based pages)
func SendHTMLError(w http.ResponseWriter, r *http.Request, err *AppError) {
	// Log the internal error message
	log.Printf("HTML Error [%s] %s: %s", r.Method, r.URL.Path, err.Message)
	if err.Details != nil {
		log.Printf("Error details: %+v", err.Details)
	}

	// For HTML responses, use standard HTTP error
	http.Error(w, err.UserMessage, err.StatusCode)
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

// IsHTMLRequest checks if the request expects HTML response
func IsHTMLRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept != "" && (contains(accept, "text/html") ||
		contains(accept, "application/xhtml+xml") ||
		contains(accept, "*/*"))
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// HandleError is a utility function that automatically chooses between JSON and HTML error responses
func HandleError(w http.ResponseWriter, r *http.Request, err *AppError) {
	if IsHTMLRequest(r) && r.Header.Get("HX-Request") == "" { // Not an HTMX request
		SendHTMLError(w, r, err)
	} else {
		SendError(w, r, err)
	}
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
