package ingest

import (
	"errors"
	"fmt"
)

var (
	// ErrTooManyFiles is returned when the file count exceeds the limit
	ErrTooManyFiles = errors.New("too many files")

	// ErrFileTooLarge is returned when a single file exceeds the size limit
	ErrFileTooLarge = errors.New("file too large")

	// ErrUploadTooLarge is returned when the total upload size exceeds the limit
	ErrUploadTooLarge = errors.New("total upload size too large")
)

// UploadLimits defines the limits for file uploads
type UploadLimits struct {
	MaxFiles      int   // Maximum number of files per request
	MaxFileBytes  int64 // Maximum size of a single file in bytes
	MaxTotalBytes int64 // Maximum total upload size in bytes
}

// DefaultUploadLimits returns the default upload limits
func DefaultUploadLimits() UploadLimits {
	return UploadLimits{
		MaxFiles:      20,
		MaxFileBytes:  1 * 1024 * 1024, // 1MB
		MaxTotalBytes: 5 * 1024 * 1024, // 5MB
	}
}

// NewUploadLimits creates upload limits from configuration
func NewUploadLimits(maxFiles int, maxFileBytes, maxTotalBytes int64) UploadLimits {
	return UploadLimits{
		MaxFiles:      maxFiles,
		MaxFileBytes:  maxFileBytes,
		MaxTotalBytes: maxTotalBytes,
	}
}

// ValidateFileCount checks if the file count is within limits
func (l UploadLimits) ValidateFileCount(count int) error {
	if count > l.MaxFiles {
		return fmt.Errorf("%w: got %d files, limit is %d", ErrTooManyFiles, count, l.MaxFiles)
	}
	return nil
}

// ValidateFileSize checks if a single file size is within limits
func (l UploadLimits) ValidateFileSize(size int64, filename string) error {
	if size > l.MaxFileBytes {
		return fmt.Errorf("%w: file %s is %d bytes, limit is %d bytes", ErrFileTooLarge, filename, size, l.MaxFileBytes)
	}
	return nil
}

// ValidateTotalSize checks if the total upload size is within limits
func (l UploadLimits) ValidateTotalSize(size int64) error {
	if size > l.MaxTotalBytes {
		return fmt.Errorf("%w: total size is %d bytes, limit is %d bytes", ErrUploadTooLarge, size, l.MaxTotalBytes)
	}
	return nil
}
