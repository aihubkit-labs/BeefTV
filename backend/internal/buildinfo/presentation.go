package buildinfo

import (
	"strings"
	"sync/atomic"
)

var displayName atomic.Value

// DisplayName is live UI copy; ProductName remains the installed package identity.
func DisplayName() string {
	if name, ok := displayName.Load().(string); ok && name != "" {
		return name
	}
	return ProductName
}

// SetDisplayName is an optional distribution hook. Empty restores the package name.
func SetDisplayName(name string) { displayName.Store(strings.TrimSpace(name)) }

// DisplayText expands only the explicit product token in host-authored copy.
// It must not rewrite user content, URLs, identifiers or third-party attribution.
func DisplayText(template string) string {
	return strings.ReplaceAll(template, "{{productName}}", DisplayName())
}

type Error string

func (e Error) Error() string { return DisplayText(string(e)) }
