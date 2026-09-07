package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	// PinFileMode is the durable pin permission (issue #20).
	PinFileMode = 0o600
	// PinDirMode is used when creating ~/.deadbugz/pins.
	PinDirMode = 0o700
)

// ExpandHome resolves a leading ~ or ~/ so IDE-spawned wraps can use
// ~/.deadbugz/pins/<name>.json without a login shell.
func ExpandHome(p string) string {
	if p == "" {
		return ""
	}
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, "~"+string(os.PathSeparator)) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~/"), "~"+string(os.PathSeparator)))
}

// SanitizeServerName keeps pin filenames stable: [A-Za-z0-9._-].
func SanitizeServerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "default"
	}
	return out
}

// DefaultPinDir is ~/.deadbugz/pins.
func DefaultPinDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".deadbugz", "pins")
	}
	return filepath.Join(home, ".deadbugz", "pins")
}

// DefaultPinPath is ~/.deadbugz/pins/<server>.json (survey: filesystem / fetch).
func DefaultPinPath(serverName string) string {
	name := SanitizeServerName(serverName)
	if name == "" {
		name = "default"
	}
	return filepath.Join(DefaultPinDir(), name+".json")
}

// DefaultApproveFile is ~/.deadbugz/approve/<server> (one-shot token).
func DefaultApproveFile(serverName string) string {
	name := SanitizeServerName(serverName)
	if name == "" {
		name = "default"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".deadbugz", "approve", name)
	}
	return filepath.Join(home, ".deadbugz", "approve", name)
}

// PendingPath is the fail-closed candidate written beside the pin.
func PendingPath(pinPath string) string {
	if pinPath == "" {
		return ""
	}
	return pinPath + ".pending"
}
