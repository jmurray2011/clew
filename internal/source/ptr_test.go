package source

import (
	"testing"
)

func TestParsePtrType(t *testing.T) {
	tests := []struct {
		name string
		ptr  string
		want PtrType
	}{
		{
			name: "local file pointer",
			ptr:  "file:///var/log/app.log#42",
			want: PtrTypeLocal,
		},
		{
			name: "local file no line number",
			ptr:  "file:///var/log/app.log",
			want: PtrTypeLocal,
		},
		{
			name: "s3 pointer",
			ptr:  "s3://mybucket/logs/app.log#1024",
			want: PtrTypeS3,
		},
		{
			name: "s3 pointer no offset",
			ptr:  "s3://mybucket/logs/app.log",
			want: PtrTypeS3,
		},
		{
			name: "cloudwatch pointer (base64-like)",
			ptr:  "CmAKJgoiMzIxMDk4NzY1NDMyOi9hd3MvbGFtYmRhL215LWZ1bmN0aW9u",
			want: PtrTypeCloudWatch,
		},
		{
			name: "cloudwatch pointer short",
			ptr:  "ABC123xyz",
			want: PtrTypeCloudWatch,
		},
		{
			name: "empty string",
			ptr:  "",
			want: PtrTypeUnknown,
		},
		{
			name: "unknown scheme",
			ptr:  "http://example.com",
			want: PtrTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePtrType(tt.ptr)
			if got != tt.want {
				t.Errorf("ParsePtrType(%q) = %v, want %v", tt.ptr, got, tt.want)
			}
		})
	}
}

func TestMakeLocalPtr(t *testing.T) {
	tests := []struct {
		name     string
		filepath string
		lineNum  int
		want     string
	}{
		{
			name:     "basic path",
			filepath: "/var/log/app.log",
			lineNum:  42,
			want:     "file:///var/log/app.log#42",
		},
		{
			name:     "zero line number",
			filepath: "/var/log/app.log",
			lineNum:  0,
			want:     "file:///var/log/app.log#0",
		},
		{
			name:     "path with spaces",
			filepath: "/var/log/my app.log",
			lineNum:  1,
			want:     "file:///var/log/my app.log#1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeLocalPtr(tt.filepath, tt.lineNum)
			if got != tt.want {
				t.Errorf("MakeLocalPtr(%q, %d) = %q, want %q", tt.filepath, tt.lineNum, got, tt.want)
			}
		})
	}
}

func TestParseLocalPtr(t *testing.T) {
	tests := []struct {
		name         string
		ptr          string
		wantOK       bool
		wantFilePath string
		wantLineNum  int
	}{
		{
			name:         "valid pointer",
			ptr:          "file:///var/log/app.log#42",
			wantOK:       true,
			wantFilePath: "/var/log/app.log",
			wantLineNum:  42,
		},
		{
			name:         "no line number",
			ptr:          "file:///var/log/app.log",
			wantOK:       true,
			wantFilePath: "/var/log/app.log",
			wantLineNum:  0,
		},
		{
			name:   "not a file pointer",
			ptr:    "s3://bucket/key",
			wantOK: false,
		},
		{
			name:   "cloudwatch pointer",
			ptr:    "CmAKJgo",
			wantOK: false,
		},
		{
			name:   "invalid fragment",
			ptr:    "file:///var/log/app.log#notanumber",
			wantOK: false,
		},
		{
			name:   "empty path",
			ptr:    "file://#42",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := ParseLocalPtr(tt.ptr)
			if ok != tt.wantOK {
				t.Errorf("ParseLocalPtr(%q) ok = %v, want %v", tt.ptr, ok, tt.wantOK)
				return
			}
			if !tt.wantOK {
				return
			}
			if info.FilePath != tt.wantFilePath {
				t.Errorf("FilePath = %q, want %q", info.FilePath, tt.wantFilePath)
			}
			if info.LineNum != tt.wantLineNum {
				t.Errorf("LineNum = %d, want %d", info.LineNum, tt.wantLineNum)
			}
		})
	}
}

func TestMakeS3Ptr(t *testing.T) {
	tests := []struct {
		name   string
		bucket string
		key    string
		offset int64
		want   string
	}{
		{
			name:   "basic",
			bucket: "mybucket",
			key:    "logs/app.log",
			offset: 1024,
			want:   "s3://mybucket/logs/app.log#1024",
		},
		{
			name:   "zero offset",
			bucket: "mybucket",
			key:    "logs/app.log",
			offset: 0,
			want:   "s3://mybucket/logs/app.log#0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeS3Ptr(tt.bucket, tt.key, tt.offset)
			if got != tt.want {
				t.Errorf("MakeS3Ptr(%q, %q, %d) = %q, want %q", tt.bucket, tt.key, tt.offset, got, tt.want)
			}
		})
	}
}

func TestParseS3Ptr(t *testing.T) {
	tests := []struct {
		name       string
		ptr        string
		wantOK     bool
		wantBucket string
		wantKey    string
		wantOffset int64
	}{
		{
			name:       "valid pointer",
			ptr:        "s3://mybucket/logs/app.log#1024",
			wantOK:     true,
			wantBucket: "mybucket",
			wantKey:    "logs/app.log",
			wantOffset: 1024,
		},
		{
			name:       "no offset",
			ptr:        "s3://mybucket/logs/app.log",
			wantOK:     true,
			wantBucket: "mybucket",
			wantKey:    "logs/app.log",
			wantOffset: 0,
		},
		{
			name:   "not an s3 pointer",
			ptr:    "file:///var/log/app.log",
			wantOK: false,
		},
		{
			name:   "no bucket",
			ptr:    "s3:///key",
			wantOK: false,
		},
		{
			name:   "no key",
			ptr:    "s3://bucket/",
			wantOK: false,
		},
		{
			name:   "invalid offset",
			ptr:    "s3://bucket/key#notanumber",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := ParseS3Ptr(tt.ptr)
			if ok != tt.wantOK {
				t.Errorf("ParseS3Ptr(%q) ok = %v, want %v", tt.ptr, ok, tt.wantOK)
				return
			}
			if !tt.wantOK {
				return
			}
			if info.Bucket != tt.wantBucket {
				t.Errorf("Bucket = %q, want %q", info.Bucket, tt.wantBucket)
			}
			if info.Key != tt.wantKey {
				t.Errorf("Key = %q, want %q", info.Key, tt.wantKey)
			}
			if info.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", info.Offset, tt.wantOffset)
			}
		})
	}
}

func TestLocalPtrRoundTrip(t *testing.T) {
	// Test that Make and Parse are inverses
	filepath := "/var/log/myapp/application.log"
	lineNum := 12345

	ptr := MakeLocalPtr(filepath, lineNum)
	info, ok := ParseLocalPtr(ptr)

	if !ok {
		t.Fatalf("ParseLocalPtr failed on pointer created by MakeLocalPtr: %s", ptr)
	}
	if info.FilePath != filepath {
		t.Errorf("FilePath = %q, want %q", info.FilePath, filepath)
	}
	if info.LineNum != lineNum {
		t.Errorf("LineNum = %d, want %d", info.LineNum, lineNum)
	}
}

func TestS3PtrRoundTrip(t *testing.T) {
	// Test that Make and Parse are inverses
	bucket := "my-log-bucket"
	key := "2025/01/15/application.log.gz"
	offset := int64(65536)

	ptr := MakeS3Ptr(bucket, key, offset)
	info, ok := ParseS3Ptr(ptr)

	if !ok {
		t.Fatalf("ParseS3Ptr failed on pointer created by MakeS3Ptr: %s", ptr)
	}
	if info.Bucket != bucket {
		t.Errorf("Bucket = %q, want %q", info.Bucket, bucket)
	}
	if info.Key != key {
		t.Errorf("Key = %q, want %q", info.Key, key)
	}
	if info.Offset != offset {
		t.Errorf("Offset = %d, want %d", info.Offset, offset)
	}
}
