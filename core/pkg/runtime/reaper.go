package runtime

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Bundle cleanup constants
const (
	// BundleDir is the base directory for OCI bundles
	BundleDir = "/tmp/orpheus-bundles"

	// MaxBundleAge is the maximum age for bundle directories before cleanup
	MaxBundleAge = 1 * time.Hour

	// ContainerPrefix is the prefix for orpheus container IDs
	ContainerPrefix = "agent-"
)

// CleanupOrphanedContainers removes stale containers and bundles on startup.
// This should be called during server/worker initialization for crash safety.
//
// Cleans up:
//  1. Orphaned runc containers (from crashed workers)
//  2. Old bundle directories (older than MaxBundleAge)
func CleanupOrphanedContainers() {
	log.Println("INFO: Running orphan cleanup...")

	// 1. Cleanup stale runc containers
	if err := cleanupRuncContainers(); err != nil {
		log.Printf("WARN: Failed to cleanup runc containers: %v", err)
	}

	// 2. Cleanup old bundle directories
	if err := cleanupOldBundles(); err != nil {
		log.Printf("WARN: Failed to cleanup bundles: %v", err)
	}

	log.Println("INFO: Orphan cleanup complete")
}

// cleanupRuncContainers removes any orphaned runc containers with our prefix
func cleanupRuncContainers() error {
	runc := NewRunc()
	if !runc.Available() {
		return nil // runc not installed, nothing to clean
	}

	// List all containers
	containers, err := runc.List()
	if err != nil {
		return err
	}

	// Delete containers with our prefix
	for _, id := range containers {
		if strings.HasPrefix(id, ContainerPrefix) {
			if err := runc.Delete(id); err == nil {
				log.Printf("INFO: Cleaned orphaned container: %s", id)
			} else {
				log.Printf("WARN: Failed to delete container %s: %v", id, err)
			}
		}
	}

	return nil
}

// cleanupOldBundles removes bundle directories older than MaxBundleAge
func cleanupOldBundles() error {
	entries, err := os.ReadDir(BundleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Bundle dir doesn't exist, nothing to clean
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Only clean our bundles (safety check)
		if !strings.HasPrefix(entry.Name(), ContainerPrefix) {
			continue
		}

		bundlePath := filepath.Join(BundleDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			log.Printf("WARN: Failed to stat bundle %s: %v", entry.Name(), err)
			continue
		}

		// Remove if older than MaxBundleAge
		if time.Since(info.ModTime()) > MaxBundleAge {
			if err := os.RemoveAll(bundlePath); err == nil {
				log.Printf("INFO: Cleaned old bundle: %s", entry.Name())
			} else {
				log.Printf("WARN: Failed to remove bundle %s: %v", entry.Name(), err)
			}
		}
	}

	return nil
}

// CleanupAllBundles removes all bundle directories (for server shutdown)
func CleanupAllBundles() error {
	return os.RemoveAll(BundleDir)
}
