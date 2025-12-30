package assets

import "fmt"

// Error codes for asset operations
type ErrorCode string

const (
	ErrorCodeNotFound         ErrorCode = "NotFound"
	ErrorCodeAlreadyExists    ErrorCode = "AlreadyExists"
	ErrorCodeInvalidCursor    ErrorCode = "InvalidCursor"
	ErrorCodeTimeout          ErrorCode = "Timeout"
	ErrorCodeFailedToFetch    ErrorCode = "FailedToFetch"
	ErrorCodeFailedToWrite    ErrorCode = "FailedToWrite"
	ErrorCodeFailedToDelete   ErrorCode = "FailedToDelete"
	ErrorCodeAssetNotUploaded ErrorCode = "AssetNotUploaded"
	ErrorCodeFileTooLarge     ErrorCode = "FileTooLarge"
)

// AssetError represents an error in asset operations
type AssetError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *AssetError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AssetError) Unwrap() error {
	return e.Err
}

// Error constructors

func NewNotFoundError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeNotFound,
		Message: message,
		Err:     err,
	}
}

func NewAlreadyExistsError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeAlreadyExists,
		Message: message,
		Err:     err,
	}
}

func NewInvalidCursorError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeInvalidCursor,
		Message: message,
		Err:     err,
	}
}

func NewTimeoutError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeTimeout,
		Message: message,
	}
}

func NewFailedToFetchError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFailedToFetch,
		Message: message,
		Err:     err,
	}
}

func NewFailedToWriteError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFailedToWrite,
		Message: message,
		Err:     err,
	}
}

func NewFailedToDeleteError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFailedToDelete,
		Message: message,
		Err:     err,
	}
}

func NewAssetNotUploadedError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeAssetNotUploaded,
		Message: message,
	}
}

func NewFileTooLargeError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFileTooLarge,
		Message: message,
	}
}

// IsNotFoundError checks if the error is a not found error
func IsNotFoundError(err error) bool {
	if assetErr, ok := err.(*AssetError); ok {
		return assetErr.Code == ErrorCodeNotFound
	}
	return false
}

// IsAlreadyExistsError checks if the error is an already exists error
func IsAlreadyExistsError(err error) bool {
	if assetErr, ok := err.(*AssetError); ok {
		return assetErr.Code == ErrorCodeAlreadyExists
	}
	return false
}

// IsInvalidCursorError checks if the error is an invalid cursor error
func IsInvalidCursorError(err error) bool {
	if assetErr, ok := err.(*AssetError); ok {
		return assetErr.Code == ErrorCodeInvalidCursor
	}
	return false
}
