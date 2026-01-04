package assets

import (
	"errors"
	"fmt"
	"testing"
)

func TestAssetError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *AssetError
		wantMsg string
	}{
		{
			name: "error without wrapped error",
			err: &AssetError{
				Code:    ErrorCodeNotFound,
				Message: "asset not found",
				Err:     nil,
			},
			wantMsg: "NotFound: asset not found",
		},
		{
			name: "error with wrapped error",
			err: &AssetError{
				Code:    ErrorCodeFailedToFetch,
				Message: "failed to fetch",
				Err:     fmt.Errorf("database error"),
			},
			wantMsg: "FailedToFetch: failed to fetch: database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestAssetError_Unwrap(t *testing.T) {
	wrappedErr := fmt.Errorf("wrapped error")
	err := &AssetError{
		Code:    ErrorCodeNotFound,
		Message: "test",
		Err:     wrappedErr,
	}

	unwrapped := err.Unwrap()
	if unwrapped != wrappedErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, wrappedErr)
	}

	errNoWrap := &AssetError{
		Code:    ErrorCodeNotFound,
		Message: "test",
		Err:     nil,
	}
	if errNoWrap.Unwrap() != nil {
		t.Error("Unwrap() should return nil when Err is nil")
	}
}

func TestNewNotFoundError(t *testing.T) {
	msg := "test not found"
	wrappedErr := fmt.Errorf("db error")
	err := NewNotFoundError(msg, wrappedErr)

	if err.Code != ErrorCodeNotFound {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeNotFound)
	}
	if err.Message != msg {
		t.Errorf("Message = %v, want %v", err.Message, msg)
	}
	if err.Err != wrappedErr {
		t.Errorf("Err = %v, want %v", err.Err, wrappedErr)
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "not found error",
			err:  NewNotFoundError("not found", nil),
			want: true,
		},
		{
			name: "wrapped not found error",
			err:  fmt.Errorf("wrapper: %w", NewNotFoundError("not found", nil)),
			want: true,
		},
		{
			name: "different asset error",
			err:  NewAlreadyExistsError("exists", nil),
			want: false,
		},
		{
			name: "regular error",
			err:  fmt.Errorf("regular error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("IsNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewAlreadyExistsError(t *testing.T) {
	msg := "already exists"
	wrappedErr := fmt.Errorf("db error")
	err := NewAlreadyExistsError(msg, wrappedErr)

	if err.Code != ErrorCodeAlreadyExists {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeAlreadyExists)
	}
	if err.Message != msg {
		t.Errorf("Message = %v, want %v", err.Message, msg)
	}
}

func TestIsAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "already exists error",
			err:  NewAlreadyExistsError("exists", nil),
			want: true,
		},
		{
			name: "wrapped already exists error",
			err:  fmt.Errorf("wrapper: %w", NewAlreadyExistsError("exists", nil)),
			want: true,
		},
		{
			name: "different error",
			err:  NewNotFoundError("not found", nil),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAlreadyExistsError(tt.err)
			if got != tt.want {
				t.Errorf("IsAlreadyExistsError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewInvalidCursorError(t *testing.T) {
	msg := "invalid cursor"
	wrappedErr := fmt.Errorf("decode error")
	err := NewInvalidCursorError(msg, wrappedErr)

	if err.Code != ErrorCodeInvalidCursor {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeInvalidCursor)
	}
}

func TestIsInvalidCursorError(t *testing.T) {
	if !IsInvalidCursorError(NewInvalidCursorError("test", nil)) {
		t.Error("IsInvalidCursorError() should return true for InvalidCursorError")
	}
	if IsInvalidCursorError(NewNotFoundError("test", nil)) {
		t.Error("IsInvalidCursorError() should return false for other errors")
	}
}

func TestNewTimeoutError(t *testing.T) {
	msg := "operation timed out"
	err := NewTimeoutError(msg)

	if err.Code != ErrorCodeTimeout {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeTimeout)
	}
	if err.Message != msg {
		t.Errorf("Message = %v, want %v", err.Message, msg)
	}
	if err.Err != nil {
		t.Errorf("Err should be nil, got %v", err.Err)
	}
}

func TestIsTimeoutError(t *testing.T) {
	if !IsTimeoutError(NewTimeoutError("timeout")) {
		t.Error("IsTimeoutError() should return true for TimeoutError")
	}
	if IsTimeoutError(errors.New("regular error")) {
		t.Error("IsTimeoutError() should return false for other errors")
	}
}

func TestNewFailedToFetchError(t *testing.T) {
	msg := "failed to fetch"
	wrappedErr := fmt.Errorf("db error")
	err := NewFailedToFetchError(msg, wrappedErr)

	if err.Code != ErrorCodeFailedToFetch {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeFailedToFetch)
	}
}

func TestIsFailedToFetchError(t *testing.T) {
	if !IsFailedToFetchError(NewFailedToFetchError("test", nil)) {
		t.Error("IsFailedToFetchError() should return true for FailedToFetchError")
	}
}

func TestNewFailedToWriteError(t *testing.T) {
	msg := "failed to write"
	wrappedErr := fmt.Errorf("write error")
	err := NewFailedToWriteError(msg, wrappedErr)

	if err.Code != ErrorCodeFailedToWrite {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeFailedToWrite)
	}
}

func TestIsFailedToWriteError(t *testing.T) {
	if !IsFailedToWriteError(NewFailedToWriteError("test", nil)) {
		t.Error("IsFailedToWriteError() should return true for FailedToWriteError")
	}
}

func TestNewFailedToDeleteError(t *testing.T) {
	msg := "failed to delete"
	wrappedErr := fmt.Errorf("delete error")
	err := NewFailedToDeleteError(msg, wrappedErr)

	if err.Code != ErrorCodeFailedToDelete {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeFailedToDelete)
	}
}

func TestIsFailedToDeleteError(t *testing.T) {
	if !IsFailedToDeleteError(NewFailedToDeleteError("test", nil)) {
		t.Error("IsFailedToDeleteError() should return true for FailedToDeleteError")
	}
}

func TestNewAssetNotUploadedError(t *testing.T) {
	msg := "not uploaded"
	err := NewAssetNotUploadedError(msg)

	if err.Code != ErrorCodeAssetNotUploaded {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeAssetNotUploaded)
	}
	if err.Message != msg {
		t.Errorf("Message = %v, want %v", err.Message, msg)
	}
}

func TestIsAssetNotUploadedError(t *testing.T) {
	if !IsAssetNotUploadedError(NewAssetNotUploadedError("test")) {
		t.Error("IsAssetNotUploadedError() should return true for AssetNotUploadedError")
	}
}

func TestNewFileTooLargeError(t *testing.T) {
	msg := "file too large"
	err := NewFileTooLargeError(msg)

	if err.Code != ErrorCodeFileTooLarge {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeFileTooLarge)
	}
}

func TestIsFileTooLargeError(t *testing.T) {
	if !IsFileTooLargeError(NewFileTooLargeError("test")) {
		t.Error("IsFileTooLargeError() should return true for FileTooLargeError")
	}
}

func TestNewNotAllowedToDeleteRootError(t *testing.T) {
	err := NewNotAllowedToDeleteRootError()

	if err.Code != ErrorCodeNotAllowedToDeleteRoot {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeNotAllowedToDeleteRoot)
	}
	if err.Message != "The root folder cannot be deleted" {
		t.Errorf("Message = %v, want 'The root folder cannot be deleted'", err.Message)
	}
}

func TestIsNotAllowedToDeleteRootError(t *testing.T) {
	if !IsNotAllowedToDeleteRootError(NewNotAllowedToDeleteRootError()) {
		t.Error("IsNotAllowedToDeleteRootError() should return true for NotAllowedToDeleteRootError")
	}
}

func TestNewFolderNotEmptyError(t *testing.T) {
	msg := "folder not empty"
	err := NewFolderNotEmptyError(msg)

	if err.Code != ErrorCodeFolderNotEmpty {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeFolderNotEmpty)
	}
}

func TestIsFolderNotEmptyError(t *testing.T) {
	if !IsFolderNotEmptyError(NewFolderNotEmptyError("test")) {
		t.Error("IsFolderNotEmptyError() should return true for FolderNotEmptyError")
	}
}

func TestNewParentFolderNotFoundError(t *testing.T) {
	msg := "parent not found"
	err := NewParentFolderNotFoundError(msg)

	if err.Code != ErrorCodeParentFolderNotFound {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeParentFolderNotFound)
	}
}

func TestIsParentFolderNotFoundError(t *testing.T) {
	if !IsParentFolderNotFoundError(NewParentFolderNotFoundError("test")) {
		t.Error("IsParentFolderNotFoundError() should return true for ParentFolderNotFoundError")
	}
}

func TestNewNotAFileError(t *testing.T) {
	msg := "not a file"
	err := NewNotAFileError(msg)

	if err.Code != ErrorCodeNotAFile {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeNotAFile)
	}
}

func TestIsNotAFileError(t *testing.T) {
	if !IsNotAFileError(NewNotAFileError("test")) {
		t.Error("IsNotAFileError() should return true for NotAFileError")
	}
}

func TestNewVersionConflictError(t *testing.T) {
	msg := "version conflict"
	err := NewVersionConflictError(msg)

	if err.Code != ErrorCodeVersionConflict {
		t.Errorf("Code = %v, want %v", err.Code, ErrorCodeVersionConflict)
	}
}

func TestIsVersionConflictError(t *testing.T) {
	if !IsVersionConflictError(NewVersionConflictError("test")) {
		t.Error("IsVersionConflictError() should return true for VersionConflictError")
	}
}
