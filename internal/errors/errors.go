package errors

import (
	"fmt"
	"log/slog"
	"net/http"
)

// ErrorType represents different categories of application errors
type ErrorType int

const (
	ValidationError ErrorType = iota
	DatabaseError
	NetworkError
	FileSystemError
	DHCPError
	TFTPError
	ConfigurationError
	AuthenticationError
)

// AppError represents application-specific errors with context
type AppError struct {
	Type    ErrorType
	Op      string                 // Operation that failed
	Err     error                  // Original error
	Message string                 // User-friendly message
	Code    int                    // HTTP status code
	Context map[string]interface{} // Additional context
}

func (e *AppError) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// String returns the error type as a string for logging
func (et ErrorType) String() string {
	switch et {
	case ValidationError:
		return "validation"
	case DatabaseError:
		return "database"
	case NetworkError:
		return "network"
	case FileSystemError:
		return "filesystem"
	case DHCPError:
		return "dhcp"
	case TFTPError:
		return "tftp"
	case ConfigurationError:
		return "configuration"
	case AuthenticationError:
		return "authentication"
	default:
		return "unknown"
	}
}

// Error constructors for different types

// NewValidationError creates a new validation error
func NewValidationError(op string, err error) *AppError {
	return &AppError{
		Type:    ValidationError,
		Op:      op,
		Err:     err,
		Message: err.Error(),
		Code:    http.StatusBadRequest,
	}
}

// NewDatabaseError creates a new database error
func NewDatabaseError(op string, err error) *AppError {
	return &AppError{
		Type:    DatabaseError,
		Op:      op,
		Err:     err,
		Message: err.Error(),
		Code:    http.StatusInternalServerError,
	}
}

// NewNetworkError creates a new network error
func NewNetworkError(op string, err error) *AppError {
	return &AppError{
		Type:    NetworkError,
		Op:      op,
		Err:     err,
		Message: "Network operation failed",
		Code:    http.StatusServiceUnavailable,
	}
}

// NewFileSystemError creates a new filesystem error
func NewFileSystemError(op string, err error) *AppError {
	return &AppError{
		Type:    FileSystemError,
		Op:      op,
		Err:     err,
		Message: "File system operation failed",
		Code:    http.StatusInternalServerError,
	}
}

// NewDHCPError creates a new DHCP-specific error
func NewDHCPError(op string, err error) *AppError {
	return &AppError{
		Type:    DHCPError,
		Op:      op,
		Err:     err,
		Message: "DHCP operation failed",
		Code:    http.StatusServiceUnavailable,
	}
}

// NewTFTPError creates a new TFTP-specific error
func NewTFTPError(op string, err error) *AppError {
	return &AppError{
		Type:    TFTPError,
		Op:      op,
		Err:     err,
		Message: "TFTP operation failed",
		Code:    http.StatusServiceUnavailable,
	}
}

// NewConfigurationError creates a new configuration error
func NewConfigurationError(op string, err error) *AppError {
	return &AppError{
		Type:    ConfigurationError,
		Op:      op,
		Err:     err,
		Message: "Configuration error",
		Code:    http.StatusInternalServerError,
	}
}

// WithContext adds context to an existing AppError
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// LogError logs an AppError with appropriate context
func LogError(logger *slog.Logger, err *AppError) {
	logArgs := []interface{}{
		slog.String("type", err.Type.String()),
		slog.String("operation", err.Op),
		slog.Int("code", err.Code),
	}

	// Add context attributes if available
	for k, v := range err.Context {
		logArgs = append(logArgs, slog.Any(k, v))
	}

	logger.Error(err.Message, logArgs...)
}

// HandleHTTPError sends appropriate HTTP error response and logs the error
func HandleHTTPError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var appErr *AppError

	// Check if it's already an AppError
	if IsAppError(err, &appErr) {
		LogError(logger, appErr)
		http.Error(w, appErr.Message, appErr.Code)
		return
	}

	// Handle unknown errors
	logger.Error("Unhandled error", slog.String("error", err.Error()))
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

// IsAppError checks if an error is an AppError and extracts it
func IsAppError(err error, target **AppError) bool {
	if appErr, ok := err.(*AppError); ok {
		*target = appErr
		return true
	}
	return false
}

// Wrap wraps an error with additional context
func Wrap(err error, op string) error {
	if err == nil {
		return nil
	}

	var appErr *AppError
	if IsAppError(err, &appErr) {
		return &AppError{
			Type:    appErr.Type,
			Op:      op + " -> " + appErr.Op,
			Err:     appErr.Err,
			Message: appErr.Message,
			Code:    appErr.Code,
			Context: appErr.Context,
		}
	}

	// Wrap non-AppErrors as generic errors
	return &AppError{
		Type:    ValidationError,
		Op:      op,
		Err:     err,
		Message: "Operation failed",
		Code:    http.StatusInternalServerError,
	}
}
