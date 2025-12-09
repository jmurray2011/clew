package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateURISyntax(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid cloudwatch URI",
			uri:     "cloudwatch:///log-group?profile=prod&region=us-east-1",
			wantErr: false,
		},
		{
			name:    "valid file URI",
			uri:     "file:///var/log/app.log",
			wantErr: false,
		},
		{
			name:    "@ instead of ? for query params",
			uri:     "cloudwatch:///log-group@profile=prod",
			wantErr: true,
			errMsg:  "use '?' for query parameters, not '@'",
		},
		{
			name:    "@ instead of ? with multiple params",
			uri:     "cloudwatch:///log-group@profile=prod&region=us-east-1",
			wantErr: true,
			errMsg:  "use '?' for query parameters, not '@'",
		},
		{
			name:    "missing scheme with triple slash",
			uri:     "///log-group?profile=prod",
			wantErr: true,
			errMsg:  "missing scheme",
		},
		{
			name:    "valid s3 URI with @ in path (not query)",
			uri:     "s3://bucket/path/file.log",
			wantErr: false,
		},
		{
			name:    "email-like @ in authority is allowed",
			uri:     "ssh://user@host/path",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURISyntax(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateURISyntax(%q) = nil, want error containing %q", tt.uri, tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateURISyntax(%q) error = %v, want error containing %q", tt.uri, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateURISyntax(%q) = %v, want nil", tt.uri, err)
				}
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get cwd: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde only",
			path: "~",
			want: home,
		},
		{
			name: "tilde with subpath",
			path: "~/logs/app.log",
			want: filepath.Join(home, "logs/app.log"),
		},
		{
			name: "relative path dot",
			path: "./app.log",
			want: filepath.Join(cwd, "app.log"),
		},
		{
			name: "relative path dot subdir",
			path: "./logs/app.log",
			want: filepath.Join(cwd, "logs/app.log"),
		},
		{
			name: "relative path dotdot",
			path: "../app.log",
			want: filepath.Join(filepath.Dir(cwd), "app.log"),
		},
		{
			name: "absolute path unchanged",
			path: "/var/log/app.log",
			want: "/var/log/app.log",
		},
		{
			name: "tilde with glob",
			path: "~/logs/*.log",
			want: filepath.Join(home, "logs/*.log"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandPath_PreservesGlobs(t *testing.T) {
	home, _ := os.UserHomeDir()

	// Test that glob patterns are preserved through expansion
	tests := []struct {
		path string
		want string
	}{
		{"~/*.log", filepath.Join(home, "*.log")},
		{"~/logs/**/*.log", filepath.Join(home, "logs/**/*.log")},
		{"./*.log", ""}, // will be absolute, just check it contains the glob
	}

	for _, tt := range tests {
		got := expandPath(tt.path)
		if tt.want != "" && got != tt.want {
			t.Errorf("expandPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
		// For relative paths, just verify the glob is preserved
		if strings.HasPrefix(tt.path, "./") {
			if !strings.HasSuffix(got, "*.log") {
				t.Errorf("expandPath(%q) = %q, expected to end with *.log", tt.path, got)
			}
		}
	}
}
