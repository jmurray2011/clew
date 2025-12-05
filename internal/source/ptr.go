package source

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Pointer format conventions:
// - CloudWatch: raw @ptr string (base64-like, e.g., "CmAKJgo...")
// - Local: "file:///path/to/file#linenum"
// - S3: "s3://bucket/key#offset"

// PtrType represents the type of a log pointer.
type PtrType string

const (
	PtrTypeCloudWatch PtrType = "cloudwatch"
	PtrTypeLocal      PtrType = "local"
	PtrTypeS3         PtrType = "s3"
	PtrTypeUnknown    PtrType = "unknown"
)

// ParsePtrType determines the source type from a pointer string.
func ParsePtrType(ptr string) PtrType {
	if strings.HasPrefix(ptr, "file://") {
		return PtrTypeLocal
	}
	if strings.HasPrefix(ptr, "s3://") {
		return PtrTypeS3
	}
	// CloudWatch @ptr values are base64-like strings without a scheme
	// They typically start with uppercase letters and contain alphanumeric chars
	if len(ptr) > 0 && !strings.Contains(ptr, "://") {
		return PtrTypeCloudWatch
	}
	return PtrTypeUnknown
}

// LocalPtrInfo contains parsed information from a local file pointer.
type LocalPtrInfo struct {
	FilePath string
	LineNum  int
}

// MakeLocalPtr creates a local file pointer from a file path and line number.
func MakeLocalPtr(filepath string, lineNum int) string {
	return fmt.Sprintf("file://%s#%d", filepath, lineNum)
}

// ParseLocalPtr extracts file path and line number from a local pointer.
// Returns the parsed info and true if successful, or zero value and false if not a valid local pointer.
func ParseLocalPtr(ptr string) (LocalPtrInfo, bool) {
	if !strings.HasPrefix(ptr, "file://") {
		return LocalPtrInfo{}, false
	}

	// Parse as URL to handle the fragment
	u, err := url.Parse(ptr)
	if err != nil {
		return LocalPtrInfo{}, false
	}

	filepath := u.Path
	if filepath == "" {
		return LocalPtrInfo{}, false
	}

	lineNum := 0
	if u.Fragment != "" {
		n, err := strconv.Atoi(u.Fragment)
		if err != nil {
			return LocalPtrInfo{}, false
		}
		lineNum = n
	}

	return LocalPtrInfo{
		FilePath: filepath,
		LineNum:  lineNum,
	}, true
}

// S3PtrInfo contains parsed information from an S3 pointer.
type S3PtrInfo struct {
	Bucket string
	Key    string
	Offset int64
}

// MakeS3Ptr creates an S3 pointer from bucket, key, and byte offset.
func MakeS3Ptr(bucket, key string, offset int64) string {
	return fmt.Sprintf("s3://%s/%s#%d", bucket, key, offset)
}

// ParseS3Ptr extracts bucket, key, and offset from an S3 pointer.
func ParseS3Ptr(ptr string) (S3PtrInfo, bool) {
	if !strings.HasPrefix(ptr, "s3://") {
		return S3PtrInfo{}, false
	}

	u, err := url.Parse(ptr)
	if err != nil {
		return S3PtrInfo{}, false
	}

	bucket := u.Host
	key := strings.TrimPrefix(u.Path, "/")
	if bucket == "" || key == "" {
		return S3PtrInfo{}, false
	}

	var offset int64
	if u.Fragment != "" {
		n, err := strconv.ParseInt(u.Fragment, 10, 64)
		if err != nil {
			return S3PtrInfo{}, false
		}
		offset = n
	}

	return S3PtrInfo{
		Bucket: bucket,
		Key:    key,
		Offset: offset,
	}, true
}
