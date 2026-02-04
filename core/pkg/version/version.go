package version

import (
	"os"
	"strings"
)

// Read returns the Orpheus version from the VERSION file.
// Searches multiple locations and falls back to default if not found.
func Read() string {
	paths := []string{
		"VERSION",
		"../VERSION",
		"../../VERSION",
		"/usr/local/share/orpheus/VERSION",
	}

	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	return "aurora-0.1.3"
}
