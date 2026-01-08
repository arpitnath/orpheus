//@CONFIG_TYPES
export interface ServerConfig {
  mode: 'unix_socket' | 'tcp';
  socket_path?: string;
  url?: string;
  auth_key?: string;
}

export interface CLIConfig {
  active: string;
  servers: Record<string, ServerConfig>;
}

//@AGENT_TYPES
export interface ScalingConfig {
  min_workers: number;
  max_workers: number;
}

export interface AgentConfig {
  name: string;
  runtime: 'python3' | 'nodejs20';
  module: string;
  entrypoint: string;
  scaling?: ScalingConfig;
  env?: Record<string, string>;
  memory_mb?: number;
  timeout_seconds?: number;
}

//@API_RESPONSE_TYPES
export interface HealthResponse {
  status: 'healthy' | 'degraded' | 'unhealthy';
  uptime_seconds: number;
  version?: string;
}

export interface QueueStats {
  pending: number;
  processing: number;
  total: number;
  max_size: number;
  is_closed: boolean;
  fill_percentage: number;
}

export interface PoolInfo {
  total_workers: number;
  idle_workers: number;
  busy_workers: number;
  desired_size: number;
  last_scale_time: string;
}

export interface AgentStats {
  agent_name: string;
  created_at: string;
  // Optional - may not exist for agents without pools (backward compat)
  queue?: QueueStats;
  pool?: PoolInfo;
  derived?: {
    utilization_percentage: number;
    requests_per_worker: number;
    pool_efficiency: string;
  };
  // Backward compatibility fields for legacy agents
  pool_status?: string;  // "not_available" or "disabled"
  message?: string;
}

export interface GlobalStats {
  total_agents: number;
  total_workers: number;
  total_pending: number;
  total_processing: number;
  total_queue_size: number;
  average_utilization: number;
  agents_with_pools: number;
  agents_without_pools: number;
}

export interface StatsResponse {
  agents: AgentStats[];
  global: GlobalStats;
  timestamp: string;
}

export interface DependencyInfo {
  installed: boolean;
  runtime: string;
  source?: string; // requirements.txt, package.json
}

export interface DeployResponse {
  agent_name: string;
  status: string;
  endpoints: {
    http: string;
    mcp?: string;
  };
  size_mb?: number;
  deployed_at?: string;
  dependencies?: DependencyInfo;
  // For TUI compatibility
  success?: boolean;
  message?: string;
}

export interface InvokeResponse {
  // Daemon format
  status?: string;
  output?: unknown;
  raw_output?: string;
  duration_ms?: number;
  // Legacy format for compatibility
  success?: boolean;
  result?: unknown;
  error?: string;
  execution_time_ms?: number;
}

export interface AgentListItem {
  name: string;
  runtime: string;
  workers: number;
  status: 'running' | 'idle' | 'stopped';
  deployed_at?: string;
}

export interface AgentDetails {
  name: string;
  runtime: string;
  module: string;
  entrypoint: string;
  status: 'running' | 'idle' | 'stopped';
  workers: number;
  scaling?: {
    min_workers: number;
    max_workers: number;
  };
  endpoints: {
    http: string;
    mcp?: string;
  };
  deployed_at?: string;
  env?: Record<string, string>;
  memory_mb?: number;
  timeout_seconds?: number;
}

export interface ContainerInfo {
  id: string;
  agent: string;
  status: 'running' | 'stopped' | 'exited';
  created: string;
  pid?: number;
}

//@CLIENT_TYPES
export interface OrpheusClient {
  health(): Promise<HealthResponse>;
  stats(agentName?: string): Promise<StatsResponse>;
  deploy(agentPath: string, options?: DeployOptions): Promise<DeployResponse>;
  invoke(agentName: string, input: unknown): Promise<InvokeResponse>;
  undeploy(agentName: string): Promise<{ success: boolean; message?: string }>;
  list(): Promise<AgentListItem[]>;
  inspect(agentName: string): Promise<AgentDetails>;
  workspaceInfo(agentName: string): Promise<WorkspaceInfoResponse>;
  workspaceClean(agentName: string): Promise<WorkspaceCleanResponse>;
  close(): void;
}

export interface DeployOptions {
  force?: boolean;
  remote?: boolean;
  config?: string;
  env?: string[];
}

//@CLI_OUTPUT_TYPES
export interface StatusData {
  daemon: {
    status: 'running' | 'stopped' | 'unknown';
    uptime?: string;
    socket?: string;
  };
  vm?: {
    status: 'running' | 'stopped' | 'unknown';
    name: string;
  };
  agents: {
    deployed: number;
    running: number;
  };
  workers: {
    total: number;
    busy: number;
    idle: number;
  };
}

export interface LogEntry {
  timestamp: string;
  level: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';
  message: string;
  agent?: string;
}

//@WORKSPACE_TYPES
export interface WorkspaceInfoResponse {
  agent_name: string;
  path: string;
  size_bytes: number;
  file_count: number;
  files?: Record<string, number>;
  exists: boolean;
}

export interface WorkspaceCleanResponse {
  status: string;
  agent_name: string;
  freed_bytes: number;
}
