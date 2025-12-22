// Package oci provides OCI bundle generation for runc container runtime.
// It converts AgentScale agent configurations to OCI-compliant runtime specifications.
//
// This package implements a minimal subset of the OCI Runtime Specification
// required for running Python agents in isolated containers.
package oci

// Spec is the OCI runtime specification (config.json)
// See: https://github.com/opencontainers/runtime-spec/blob/main/config.md
type Spec struct {
	// Version is the OCI specification version (e.g., "1.0.2")
	Version string `json:"ociVersion"`

	// Process configures the container's main process
	Process *Process `json:"process"`

	// Root configures the container's root filesystem
	Root *Root `json:"root"`

	// Mounts configures additional filesystem mounts
	Mounts []Mount `json:"mounts,omitempty"`

	// Linux contains Linux-specific configuration
	Linux *Linux `json:"linux,omitempty"`
}

// Process contains information about the container's main process
type Process struct {
	// Terminal indicates whether a terminal is attached (false for agents)
	Terminal bool `json:"terminal"`

	// User specifies the user identity for the process
	User User `json:"user"`

	// Args is the command to run (e.g., ["python3", "agent.py"])
	Args []string `json:"args"`

	// Env contains environment variables
	Env []string `json:"env,omitempty"`

	// Cwd is the current working directory
	Cwd string `json:"cwd"`

	// Capabilities specifies Linux capabilities (all empty for security)
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}

// User specifies the user identity for the container process
type User struct {
	// UID is the user ID (1000 for non-root execution)
	UID uint32 `json:"uid"`

	// GID is the group ID (1000 for non-root execution)
	GID uint32 `json:"gid"`
}

// Capabilities contains Linux capability sets
// All sets are empty to drop ALL capabilities for security
type Capabilities struct {
	// Bounding is the capability bounding set
	Bounding []string `json:"bounding"`

	// Effective is the effective capability set
	Effective []string `json:"effective"`

	// Inheritable is the inheritable capability set
	Inheritable []string `json:"inheritable"`

	// Permitted is the permitted capability set
	Permitted []string `json:"permitted"`

	// Ambient is the ambient capability set
	Ambient []string `json:"ambient"`
}

// Root contains the root filesystem configuration
type Root struct {
	// Path is the path to the root filesystem (relative to bundle or absolute)
	Path string `json:"path"`

	// Readonly makes the root filesystem read-only
	Readonly bool `json:"readonly"`
}

// Mount specifies a mount point for the container
type Mount struct {
	// Destination is the mount point inside the container
	Destination string `json:"destination"`

	// Type is the filesystem type (e.g., "proc", "tmpfs")
	Type string `json:"type"`

	// Source is the source for the mount
	Source string `json:"source"`

	// Options are mount options
	Options []string `json:"options,omitempty"`
}

// Linux contains Linux-specific container configuration
type Linux struct {
	// Namespaces configures Linux namespaces for isolation
	// We use: pid, mount, ipc, uts
	// We do NOT use: network (agents need host network for API access)
	Namespaces []Namespace `json:"namespaces,omitempty"`

	// Resources configures resource limits via cgroups
	Resources *LinuxResources `json:"resources,omitempty"`
}

// Namespace configures a Linux namespace
type Namespace struct {
	// Type is the namespace type (pid, mount, ipc, uts, network, user, cgroup)
	Type string `json:"type"`

	// Path is an optional path to an existing namespace
	Path string `json:"path,omitempty"`
}

// LinuxResources contains cgroup resource configuration
type LinuxResources struct {
	// Memory contains memory limit configuration
	Memory *LinuxMemory `json:"memory,omitempty"`

	// Pids contains process limit configuration
	Pids *LinuxPids `json:"pids,omitempty"`
}

// LinuxMemory contains memory cgroup configuration
type LinuxMemory struct {
	// Limit is the hard memory limit in bytes (memory.max in cgroups v2)
	// When exceeded, the process is OOM killed
	Limit *int64 `json:"limit,omitempty"`
}

// LinuxPids contains pids cgroup configuration
type LinuxPids struct {
	// Limit is the maximum number of processes/threads
	Limit int64 `json:"limit"`
}
