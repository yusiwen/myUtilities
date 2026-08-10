package installer

import "testing"

func TestGetArch(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"amd64", "tool_linux_amd64.tar.gz", "amd64"},
		{"x86_64", "tool-linux-x86_64.tar.gz", "amd64"},
		{"arm64", "tool_darwin_arm64.zip", "arm64"},
		{"aarch64", "tool-linux-aarch64.tar.gz", "arm64"},
		{"386", "tool_linux_386.tar.gz", "386"},
		{"686", "tool_linux_686.tar.gz", "386"},
		{"suffix64", "tool_linux64.tar.gz", "amd64"},
		{"suffix32", "tool_linux32.tar.gz", "386"},
		// version numbers must not be mistaken for the 32/64 fallback
		{"version64-linux-amd64", "helm-v3.64.0-linux-amd64.tar.gz", "amd64"},
		{"version32-linux-amd64", "helm-v3.32.0-linux-amd64.tar.gz", "amd64"},
		{"version32-386", "helm-v3.32.0-linux-386.tar.gz", "386"},
		{"version-1.2.3", "foo-1.2.3-linux-amd64.tar.gz", "amd64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetArch(tt.in); got != tt.want {
				t.Fatalf("GetArch(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetOS(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"tool_linux_amd64.tar.gz", "linux"},
		{"tool_darwin_arm64.zip", "darwin"},
		{"tool_MacOS_x86_64.tar.gz", "darwin"},
		{"tool_osx_amd64.tar.gz", "darwin"},
		{"tool_freebsd_amd64.tar.gz", "freebsd"},
		{"tool_win_amd64.zip", "windows"},
		{"tool_windows_amd64.zip", "windows"},
		{"tool_bare.tar.gz", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := GetOS(tt.in); got != tt.want {
				t.Fatalf("GetOS(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
