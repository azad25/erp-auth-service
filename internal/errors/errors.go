package errors

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCode represents structured error codes following Google's error model
type ErrorCode int32

const (
	// Standard gRPC error codes
	ErrorCodeOK                 ErrorCode = 0
	ErrorCodeCancelled         ErrorCode = 1
	ErrorCodeUnknown           ErrorCode = 2
	ErrorCodeInvalidArgument   ErrorCode = 3
	ErrorCodeDeadlineExceeded  ErrorCode = 4
	ErrorCodeNotFound          ErrorCode = 5
	ErrorCodeAlreadyExists     ErrorCode = 6
	ErrorCodePermissionDenied  ErrorCode = 7
	ErrorCodeResourceExhausted ErrorCode = 8
	ErrorCodeFailedPrecondition ErrorCode = 9
	ErrorCodeAborted           ErrorCode = 10
	ErrorCodeOutOfRange        ErrorCode = 11
	ErrorCodeUnimplemented     ErrorCode = 12
	ErrorCodeInternal          ErrorCode = 13
	ErrorCodeUnavailable       ErrorCode = 14
	ErrorCodeDataLoss          ErrorCode = 15
	ErrorCodeUnauthenticated   ErrorCode = 16

	// Custom auth service error codes (starting from 1000)
	ErrorCodeInvalidCredentials     ErrorCode = 1000
	ErrorCodeTokenExpired          ErrorCode = 1001
	ErrorCodeTokenInvalid          ErrorCode = 1002
	ErrorCodeUserNotFound          ErrorCode = 1003
	ErrorCodeUserAlreadyExists     ErrorCode = 1004
	ErrorCodeOrganizationNotFound  ErrorCode = 1005
	ErrorCodePermissionNotFound    ErrorCode = 1006
	ErrorCodeRoleNotFound          ErrorCode = 1007
	ErrorCodePasswordTooWeak       ErrorCode = 1008
	ErrorCodeAccountLocked         ErrorCode = 1009
	ErrorCodeTwoFactorRequired     ErrorCode = 1010
	ErrorCodeInvalidTwoFactorCode  ErrorCode = 1011
	ErrorCodeRateLimitExceeded     ErrorCode = 1012
	ErrorCodeDatabaseError         ErrorCode = 1013
	ErrorCodeCacheError            ErrorCode = 1014
	ErrorCodeEventPublishError     ErrorCode = 1015
	ErrorCodeCircuitBreakerOpen    ErrorCode = 1016
	ErrorCodeRetryExhausted        ErrorCode = 1017
)

// ErrorSeverity represents the severity level of an error
type ErrorSeverity string

const (
	SeverityInfo     ErrorSeverity = "INFO"
	SeverityWarning  ErrorSeverity = "WARNING"
	SeverityError    ErrorSeverity = "ERROR"
	SeverityCritical ErrorSeverity = "CRITICAL"
)

// ErrorCategory represents the category of an error for better classification
type ErrorCategory string

const (
	CategoryAuthentication ErrorCategory = "AUTHENTICATION"
	CategoryAuthorization  ErrorCategory = "AUTHORIZATION"
	CategoryValidation     ErrorCategory = "VALIDATION"
	CategoryDatabase       ErrorCategory = "DATABASE"
	CategoryCache          ErrorCategory = "CACHE"
	CategoryNetwork        ErrorCategory = "NETWORK"
	CategoryInternal       ErrorCategory = "INTERNAL"
	CategoryExternal       ErrorCategory = "EXTERNAL"
	CategoryRateLimit      ErrorCategory = "RATE_LIMIT"
	CategoryCircuitBreaker ErrorCategory = "CIRCUIT_BREAKER"
)

// StackFrame represents a single frame in the call stack
type StackFrame struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// AuthError represents a structured error following Google's error model
type AuthError struct {
	// Core error information
	Code        ErrorCode     `json:"code"`
	Message     string        `json:"message"`
	Details     interface{}   `json:"details,omitempty"`
	
	// Context information
	TraceID     string        `json:"trace_id"`
	RequestID   string        `json:"request_id,omitempty"`
	UserID      string        `json:"user_id,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
	
	// Error classification
	Category    ErrorCategory `json:"category"`
	Severity    ErrorSeverity `json:"severity"`
	Retryable   bool          `json:"retryable"`
	
	// Stack trace information
	Stack       []StackFrame  `json:"stack,omitempty"`
	
	// Underlying error
	Cause       error         `json:"-"`
	
	// Additional metadata
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Error implements the error interface
func (e *AuthError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for error unwrapping
func (e *AuthError) Unwrap() error {
	return e.Cause
}

// ToGRPCStatus converts the AuthError to a gRPC status
func (e *AuthError) ToGRPCStatus() *status.Status {
	grpcCode := e.toGRPCCode()
	
	// Create status with message that includes additional context
	message := e.Message
	if e.TraceID != "" {
		message = fmt.Sprintf("%s (trace_id: %s)", message, e.TraceID)
	}
	
	return status.New(grpcCode, message)
}

// toGRPCCode maps AuthError codes to gRPC codes
func (e *AuthError) toGRPCCode() codes.Code {
	switch e.Code {
	case ErrorCodeOK:
		return codes.OK
	case ErrorCodeCancelled:
		return codes.Canceled
	case ErrorCodeUnknown:
		return codes.Unknown
	case ErrorCodeInvalidArgument:
		return codes.InvalidArgument
	case ErrorCodeDeadlineExceeded:
		return codes.DeadlineExceeded
	case ErrorCodeNotFound, ErrorCodeUserNotFound, ErrorCodeOrganizationNotFound:
		return codes.NotFound
	case ErrorCodeAlreadyExists, ErrorCodeUserAlreadyExists:
		return codes.AlreadyExists
	case ErrorCodePermissionDenied:
		return codes.PermissionDenied
	case ErrorCodeResourceExhausted, ErrorCodeRateLimitExceeded:
		return codes.ResourceExhausted
	case ErrorCodeFailedPrecondition:
		return codes.FailedPrecondition
	case ErrorCodeAborted:
		return codes.Aborted
	case ErrorCodeOutOfRange:
		return codes.OutOfRange
	case ErrorCodeUnimplemented:
		return codes.Unimplemented
	case ErrorCodeInternal, ErrorCodeDatabaseError, ErrorCodeCacheError:
		return codes.Internal
	case ErrorCodeUnavailable, ErrorCodeCircuitBreakerOpen:
		return codes.Unavailable
	case ErrorCodeDataLoss:
		return codes.DataLoss
	case ErrorCodeUnauthenticated, ErrorCodeInvalidCredentials, ErrorCodeTokenExpired, ErrorCodeTokenInvalid:
		return codes.Unauthenticated
	default:
		return codes.Unknown
	}
}

// IsRetryable returns whether the error is retryable
func (e *AuthError) IsRetryable() bool {
	return e.Retryable
}

// WithMetadata adds metadata to the error
func (e *AuthError) WithMetadata(key string, value interface{}) *AuthError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// WithUserID adds user ID to the error context
func (e *AuthError) WithUserID(userID string) *AuthError {
	e.UserID = userID
	return e
}

// WithRequestID adds request ID to the error context
func (e *AuthError) WithRequestID(requestID string) *AuthError {
	e.RequestID = requestID
	return e
}

// captureStack captures the current call stack
func captureStack(skip int) []StackFrame {
	var frames []StackFrame
	
	// Capture up to 10 stack frames
	for i := skip; i < skip+10; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}
		
		frames = append(frames, StackFrame{
			Function: fn.Name(),
			File:     file,
			Line:     line,
		})
	}
	
	return frames
}

// ErrorBuilder provides a fluent interface for building AuthErrors
type ErrorBuilder struct {
	error *AuthError
}

// NewError creates a new error builder
func NewError(code ErrorCode, message string) *ErrorBuilder {
	return &ErrorBuilder{
		error: &AuthError{
			Code:      code,
			Message:   message,
			Timestamp: time.Now(),
			Stack:     captureStack(2), // Skip NewError and caller
			Metadata:  make(map[string]interface{}),
		},
	}
}

// WithCause sets the underlying cause of the error
func (b *ErrorBuilder) WithCause(cause error) *ErrorBuilder {
	b.error.Cause = cause
	return b
}

// WithDetails sets additional details for the error
func (b *ErrorBuilder) WithDetails(details interface{}) *ErrorBuilder {
	b.error.Details = details
	return b
}

// WithTraceID sets the trace ID for the error
func (b *ErrorBuilder) WithTraceID(traceID string) *ErrorBuilder {
	b.error.TraceID = traceID
	return b
}

// WithCategory sets the error category
func (b *ErrorBuilder) WithCategory(category ErrorCategory) *ErrorBuilder {
	b.error.Category = category
	return b
}

// WithSeverity sets the error severity
func (b *ErrorBuilder) WithSeverity(severity ErrorSeverity) *ErrorBuilder {
	b.error.Severity = severity
	return b
}

// WithRetryable sets whether the error is retryable
func (b *ErrorBuilder) WithRetryable(retryable bool) *ErrorBuilder {
	b.error.Retryable = retryable
	return b
}

// WithMetadata adds metadata to the error
func (b *ErrorBuilder) WithMetadata(key string, value interface{}) *ErrorBuilder {
	b.error.Metadata[key] = value
	return b
}

// WithUserID adds user ID to the error context
func (b *ErrorBuilder) WithUserID(userID string) *ErrorBuilder {
	b.error.UserID = userID
	return b
}

// WithRequestID adds request ID to the error context
func (b *ErrorBuilder) WithRequestID(requestID string) *ErrorBuilder {
	b.error.RequestID = requestID
	return b
}

// Build returns the constructed AuthError
func (b *ErrorBuilder) Build() *AuthError {
	return b.error
}

// Predefined error constructors for common scenarios

// NewInvalidCredentialsError creates an invalid credentials error
func NewInvalidCredentialsError(email string) *AuthError {
	return NewError(ErrorCodeInvalidCredentials, "Invalid email or password").
		WithCategory(CategoryAuthentication).
		WithSeverity(SeverityWarning).
		WithRetryable(false).
		WithMetadata("email", email).
		Build()
}

// NewTokenExpiredError creates a token expired error
func NewTokenExpiredError(tokenType string) *AuthError {
	return NewError(ErrorCodeTokenExpired, fmt.Sprintf("%s token has expired", tokenType)).
		WithCategory(CategoryAuthentication).
		WithSeverity(SeverityInfo).
		WithRetryable(false).
		WithMetadata("token_type", tokenType).
		Build()
}

// NewUserNotFoundError creates a user not found error
func NewUserNotFoundError(identifier string) *AuthError {
	return NewError(ErrorCodeUserNotFound, "User not found").
		WithCategory(CategoryValidation).
		WithSeverity(SeverityWarning).
		WithRetryable(false).
		WithMetadata("identifier", identifier).
		Build()
}

// NewDatabaseError creates a database error
func NewDatabaseError(operation string, cause error) *AuthError {
	return NewError(ErrorCodeDatabaseError, fmt.Sprintf("Database operation failed: %s", operation)).
		WithCause(cause).
		WithCategory(CategoryDatabase).
		WithSeverity(SeverityError).
		WithRetryable(true).
		WithMetadata("operation", operation).
		Build()
}

// NewCacheError creates a cache error
func NewCacheError(operation string, cause error) *AuthError {
	return NewError(ErrorCodeCacheError, fmt.Sprintf("Cache operation failed: %s", operation)).
		WithCause(cause).
		WithCategory(CategoryCache).
		WithSeverity(SeverityWarning).
		WithRetryable(true).
		WithMetadata("operation", operation).
		Build()
}

// NewRateLimitError creates a rate limit exceeded error
func NewRateLimitError(limit int, window time.Duration) *AuthError {
	return NewError(ErrorCodeRateLimitExceeded, "Rate limit exceeded").
		WithCategory(CategoryRateLimit).
		WithSeverity(SeverityWarning).
		WithRetryable(true).
		WithMetadata("limit", limit).
		WithMetadata("window", window.String()).
		Build()
}

// NewCircuitBreakerError creates a circuit breaker open error
func NewCircuitBreakerError(service string) *AuthError {
	return NewError(ErrorCodeCircuitBreakerOpen, fmt.Sprintf("Circuit breaker is open for service: %s", service)).
		WithCategory(CategoryCircuitBreaker).
		WithSeverity(SeverityError).
		WithRetryable(true).
		WithMetadata("service", service).
		Build()
}



// IsAuthError checks if an error is an AuthError
func IsAuthError(err error) bool {
	_, ok := err.(*AuthError)
	return ok
}

// GetAuthError extracts AuthError from an error, creating one if necessary
func GetAuthError(err error) *AuthError {
	if authErr, ok := err.(*AuthError); ok {
		return authErr
	}
	
	// Create a generic AuthError for non-AuthError types
	return NewError(ErrorCodeUnknown, err.Error()).
		WithCause(err).
		WithCategory(CategoryInternal).
		WithSeverity(SeverityError).
		WithRetryable(false).
		Build()
}

// WrapError wraps an existing error with additional context
func WrapError(err error, code ErrorCode, message string) *AuthError {
	return NewError(code, message).
		WithCause(err).
		Build()
}

// ErrorFromContext extracts error information from context
func ErrorFromContext(ctx context.Context, err error) *AuthError {
	authErr := GetAuthError(err)
	
	// Extract trace ID from context if available
	if traceID := ctx.Value("trace_id"); traceID != nil {
		if traceIDStr, ok := traceID.(string); ok {
			authErr.TraceID = traceIDStr
		}
	}
	
	// Extract request ID from context if available
	if requestID := ctx.Value("request_id"); requestID != nil {
		if requestIDStr, ok := requestID.(string); ok {
			authErr.RequestID = requestIDStr
		}
	}
	
	// Extract user ID from context if available
	if userID := ctx.Value("user_id"); userID != nil {
		if userIDStr, ok := userID.(string); ok {
			authErr.UserID = userIDStr
		}
	}
	
	return authErr
}