package assets

import (
	"errors"
	"fmt"
)

// Error codes for asset operations
type ErrorCode string

const (
	ErrorCodeNotFound               ErrorCode = "NotFound"
	ErrorCodeAlreadyExists          ErrorCode = "AlreadyExists"
	ErrorCodeInvalidCursor          ErrorCode = "InvalidCursor"
	ErrorCodeTimeout                ErrorCode = "Timeout"
	ErrorCodeFailedToFetch          ErrorCode = "FailedToFetch"
	ErrorCodeFailedToWrite          ErrorCode = "FailedToWrite"
	ErrorCodeFailedToDelete         ErrorCode = "FailedToDelete"
	ErrorCodeAssetNotUploaded       ErrorCode = "AssetNotUploaded"
	ErrorCodeFileTooLarge           ErrorCode = "FileTooLarge"
	ErrorCodeFolderNotEmpty         ErrorCode = "FolderNotEmpty"
	ErrorCodeParentFolderNotFound   ErrorCode = "ParentFolderNotFound"
	ErrorCodeNotAFile               ErrorCode = "NotAFile"
	ErrorCodeVersionConflict        ErrorCode = "VersionConflict"
	ErrorCodeNotAllowedToDeleteRoot ErrorCode = "NotAllowedToDeleteRoot"
	ErrorCodeFileNotConfirmed       ErrorCode = "FileNotConfirmed"
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

func NewNotFoundError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeNotFound,
		Message: message,
		Err:     err,
	}
}

func IsNotFoundError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeNotFound
	}
	return false
}

func NewAlreadyExistsError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeAlreadyExists,
		Message: message,
		Err:     err,
	}
}

func IsAlreadyExistsError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeAlreadyExists
	}
	return false
}

func NewInvalidCursorError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeInvalidCursor,
		Message: message,
		Err:     err,
	}
}

func IsInvalidCursorError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeInvalidCursor
	}
	return false
}

func NewTimeoutError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeTimeout,
		Message: message,
	}
}

func IsTimeoutError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeTimeout
	}
	return false
}

func NewFailedToFetchError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFailedToFetch,
		Message: message,
		Err:     err,
	}
}

func IsFailedToFetchError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeFailedToFetch
	}
	return false
}

func NewFailedToWriteError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFailedToWrite,
		Message: message,
		Err:     err,
	}
}

func IsFailedToWriteError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeFailedToWrite
	}
	return false
}

func NewFailedToDeleteError(message string, err error) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFailedToDelete,
		Message: message,
		Err:     err,
	}
}

func IsFailedToDeleteError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeFailedToDelete
	}
	return false
}

func NewAssetNotUploadedError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeAssetNotUploaded,
		Message: message,
	}
}

func IsAssetNotUploadedError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeAssetNotUploaded
	}
	return false
}

func NewFileTooLargeError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFileTooLarge,
		Message: message,
	}
}

func IsFileTooLargeError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeFileTooLarge
	}
	return false
}

func NewNotAllowedToDeleteRootError() *AssetError {
	return &AssetError{
		Code:    ErrorCodeNotAllowedToDeleteRoot,
		Message: "The root folder cannot be deleted",
	}
}

func IsNotAllowedToDeleteRootError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeNotAllowedToDeleteRoot
	}
	return false
}

func NewFolderNotEmptyError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFolderNotEmpty,
		Message: message,
	}
}

func IsFolderNotEmptyError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeFolderNotEmpty
	}
	return false
}

func NewParentFolderNotFoundError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeParentFolderNotFound,
		Message: message,
	}
}

func IsParentFolderNotFoundError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeParentFolderNotFound
	}
	return false
}

func NewNotAFileError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeNotAFile,
		Message: message,
	}
}

func IsNotAFileError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeNotAFile
	}
	return false
}

func NewVersionConflictError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeVersionConflict,
		Message: message,
	}
}

func IsVersionConflictError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeVersionConflict
	}
	return false
}

func NewFileNotConfirmedError(message string) *AssetError {
	return &AssetError{
		Code:    ErrorCodeFileNotConfirmed,
		Message: message,
	}
}

func IsFileNotConfirmedError(err error) bool {
	var assetErr *AssetError
	if errors.As(err, &assetErr) {
		return assetErr.Code == ErrorCodeFileNotConfirmed
	}
	return false
}
