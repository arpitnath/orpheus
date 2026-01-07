// Package config handles loading and validating agent.yaml configuration files.
package config

import "fmt"

// ErrorCode represents specific configuration error types
type ErrorCode string

const (
	ErrCodeNotFound      ErrorCode = "CONFIG_NOT_FOUND"
	ErrCodeInvalidYAML   ErrorCode = "INVALID_YAML"
	ErrCodeMissingField  ErrorCode = "MISSING_FIELD"
	ErrCodeInvalidValue  ErrorCode = "INVALID_VALUE"
	ErrCodeFileRead      ErrorCode = "FILE_READ_ERROR"
	ErrCodeUnsupportedRT ErrorCode = "UNSUPPORTED_RUNTIME"
)

// ConfigError represents a configuration-related error
type ConfigError struct {
	Code    ErrorCode
	Message string
	Field   string // Optional: which field caused the error
	Cause   error  // Optional: underlying error
}

// Error implements the error interface
func (e *ConfigError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("[%s] %s (field: %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// NewConfigError creates a new ConfigError
func NewConfigError(code ErrorCode, message string) *ConfigError {
	return &ConfigError{
		Code:    code,
		Message: message,
	}
}

// NewFieldError creates a ConfigError for a specific field
func NewFieldError(code ErrorCode, field, message string) *ConfigError {
	return &ConfigError{
		Code:    code,
		Message: message,
		Field:   field,
	}
}

// WrapError wraps an underlying error with a ConfigError
func WrapError(code ErrorCode, message string, cause error) *ConfigError {
	return &ConfigError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
