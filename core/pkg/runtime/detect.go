package runtime

// RuncAvailable returns true if runc binary is installed and in PATH
func RuncAvailable() bool {
	runc := NewRunc()
	return runc.Available()
}

// Note: Docker support removed in Phase 6.
// macOS isolation now requires Lima VM (Phase 7+).
