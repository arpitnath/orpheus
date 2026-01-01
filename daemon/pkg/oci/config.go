package oci

import (
	"orpheus/daemon/pkg/config"
)

// Security constants for container execution
const (
	// ContainerUID is the user ID for the container process (non-root)
	// Running as non-root limits blast radius of container escape
	ContainerUID uint32 = 1000

	// ContainerGID is the group ID for the container process (non-root)
	ContainerGID uint32 = 1000

	// DefaultPidsLimit is the maximum number of processes/threads
	// Prevents fork bombs and runaway process creation
	DefaultPidsLimit int64 = 256

	// OCIVersion is the OCI specification version we target
	OCIVersion = "1.0.2"
)

// GenerateSpec creates an OCI runtime specification from agent configuration.
//
// Security hardening applied:
//   - Non-root execution (uid 1000, gid 1000)
//   - All Linux capabilities dropped (empty bounding set)
//   - No network namespace (shared host network for API access)
//   - tmpfs for /tmp and /dev (no persistence, limited device access)
//   - PID limit to prevent fork bombs
//   - Memory limit to prevent resource exhaustion
//
// Namespaces enabled:
//   - pid: Process isolation (can't see host processes)
//   - mount: Filesystem isolation (pivot_root to agent rootfs)
//   - ipc: IPC isolation (separate shared memory)
//   - uts: Hostname isolation
//
// Namespaces NOT enabled (intentionally):
//   - network: Agents need host network for LLM API calls
//   - user: Adds complexity, we use non-root UID instead
//
// Runtime support:
//   - python3: Uses /usr/local/bin/python3.10 with _entrypoint.py
//   - nodejs20: Uses /usr/local/bin/node with _entrypoint.mjs
func GenerateSpec(cfg *config.AgentConfig) *Spec {
	// Calculate memory limit in bytes
	memoryLimit := int64(cfg.MemoryLimit * 1024 * 1024)

	// Build runtime-specific process args and environment
	var args []string
	var env []string

	switch cfg.Runtime {
	case config.RuntimeNodeJS20:
		// Node.js 20 runtime
		args = []string{
			"/usr/local/bin/node",
			"/agent/_entrypoint.mjs",
		}
		env = []string{
			"PATH=/usr/local/bin:/usr/bin:/bin",
			"NODE_PATH=/packages:/agent/node_modules:/agent",
			"NODE_ENV=production",
			"HOME=/tmp", // Non-root user needs writable HOME
		}
	default:
		// Python 3 runtime (default)
		args = []string{
			"/usr/local/bin/python3.10",
			"/agent/_entrypoint.py",
		}
		env = []string{
			"PATH=/usr/local/bin:/usr/bin:/bin",
			"PYTHONPATH=/packages:/agent",
			"PYTHONUNBUFFERED=1",
			"PYTHONDONTWRITEBYTECODE=1",
			"HOME=/tmp", // Non-root user needs writable HOME
		}
	}

	// Append agent-specific environment variables
	env = append(env, cfg.Env...)

	return &Spec{
		Version: OCIVersion,
		Process: &Process{
			Terminal: false,
			User: User{
				UID: ContainerUID,
				GID: ContainerGID,
			},
			Args: args,
			Env:  env,
			Cwd:  "/agent",
			// Security: Drop ALL Linux capabilities
			Capabilities: &Capabilities{
				Bounding:    []string{},
				Effective:   []string{},
				Inheritable: []string{},
				Permitted:   []string{},
				Ambient:     []string{},
			},
		},
		Root: &Root{
			Path:     "rootfs",
			Readonly: false, // Some agents may need to write temp files
		},
		Mounts: []Mount{
			{
				Destination: "/proc",
				Type:        "proc",
				Source:      "proc",
			},
			{
				Destination: "/dev",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/tmp",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "nodev", "mode=1777"},
			},
		},
		Linux: &Linux{
			// Namespaces for isolation
			// NOTE: NO network namespace - agents share host network for API access
			Namespaces: []Namespace{
				{Type: "pid"},
				{Type: "mount"},
				{Type: "ipc"},
				{Type: "uts"},
			},
			Resources: &LinuxResources{
				Memory: &LinuxMemory{
					Limit: &memoryLimit,
				},
				Pids: &LinuxPids{
					Limit: DefaultPidsLimit,
				},
			},
		},
	}
}

// GenerateSpecWithOptions creates an OCI spec with additional customization options.
// This is useful for testing or when specific overrides are needed.
type SpecOptions struct {
	// RootfsPath overrides the default "rootfs" path
	// Use absolute path when not using symlinks
	RootfsPath string

	// MemoryLimitMB overrides the memory limit from config
	MemoryLimitMB int

	// PidsLimit overrides the default pids limit
	PidsLimit int64

	// ReadonlyRootfs makes the rootfs read-only
	ReadonlyRootfs bool

	// AdditionalEnv adds extra environment variables
	AdditionalEnv []string

	// CustomArgs overrides the default process args
	CustomArgs []string
}

// GenerateSpecWithOptions creates a spec with custom options
func GenerateSpecWithOptions(cfg *config.AgentConfig, opts *SpecOptions) *Spec {
	spec := GenerateSpec(cfg)

	if opts == nil {
		return spec
	}

	// Apply rootfs path override (use absolute path for runc)
	if opts.RootfsPath != "" {
		spec.Root.Path = opts.RootfsPath
	}

	// Apply overrides
	if opts.MemoryLimitMB > 0 {
		limit := int64(opts.MemoryLimitMB * 1024 * 1024)
		spec.Linux.Resources.Memory.Limit = &limit
	}

	if opts.PidsLimit > 0 {
		spec.Linux.Resources.Pids.Limit = opts.PidsLimit
	}

	if opts.ReadonlyRootfs {
		spec.Root.Readonly = true
	}

	if len(opts.AdditionalEnv) > 0 {
		spec.Process.Env = append(spec.Process.Env, opts.AdditionalEnv...)
	}

	if len(opts.CustomArgs) > 0 {
		spec.Process.Args = opts.CustomArgs
	}

	return spec
}
