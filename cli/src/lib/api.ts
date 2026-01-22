//@IMPORTS
import { request as httpRequest } from 'node:http';
import { request as httpsRequest, RequestOptions } from 'node:https';
import { URL } from 'node:url';
import { getActiveServer } from './config.js';
import { createTarball, calculateChecksum, uploadAgent, validateAgentPath } from './deploy.js';
import type {
  ServerConfig,
  HealthResponse,
  StatsResponse,
  DeployResponse,
  InvokeResponse,
  AgentListItem,
  AgentDetails,
  OrpheusClient,
  DeployOptions,
  WorkspaceInfoResponse,
  WorkspaceCleanResponse,
  CrashedRequestsResponse,
  ExecLogFilters,
  ExecLogsResponse,
  ExecLogStatsResponse,
} from '../types/index.js';

//@HTTP_CLIENT
async function makeRequest<T>(
  method: string,
  path: string,
  serverConfig: ServerConfig,
  body?: unknown,
  timeout: number = 600000 // 600s default for long agent executions
): Promise<T> {
  return new Promise((resolve, reject) => {
    let options: RequestOptions;
    let requestFn: typeof httpRequest;

    if (serverConfig.mode === 'unix_socket') {
      // Unix socket mode
      const socketPath = serverConfig.socket_path;
      if (!socketPath) {
        reject(new Error('Socket path not configured'));
        return;
      }

      options = {
        socketPath,
        path,
        method,
        headers: {
          'Content-Type': 'application/json',
        },
        timeout,
      };
      requestFn = httpRequest;
    } else {
      // TCP mode
      const url = serverConfig.url;
      if (!url) {
        reject(new Error('Server URL not configured'));
        return;
      }

      const parsedUrl = new URL(path, url);
      const isHttps = parsedUrl.protocol === 'https:';

      options = {
        hostname: parsedUrl.hostname,
        port: parsedUrl.port || (isHttps ? 443 : 80),
        path: parsedUrl.pathname + parsedUrl.search,
        method,
        headers: {
          'Content-Type': 'application/json',
        },
        timeout,
        agent: false, // Disable connection pooling to prevent header issues
      };
      requestFn = isHttps ? httpsRequest : httpRequest;
    }

    const req = requestFn(options, (res) => {
      let data = '';

      res.on('data', (chunk: Buffer) => {
        data += chunk.toString();
      });

      res.on('end', () => {
        if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
          try {
            const parsed = data ? JSON.parse(data) : {};
            resolve(parsed as T);
          } catch {
            resolve(data as unknown as T);
          }
        } else {
          let errorMessage = `HTTP ${res.statusCode}`;
          try {
            const errorData = JSON.parse(data);
            errorMessage = errorData.error || errorData.message || errorMessage;
          } catch {
            if (data) errorMessage = data;
          }
          reject(new Error(errorMessage));
        }
      });
    });

    req.on('error', (err: Error) => {
      if ((err as NodeJS.ErrnoException).code === 'ECONNREFUSED') {
        reject(new Error('Cannot connect to daemon. Is it running?'));
      } else if ((err as NodeJS.ErrnoException).code === 'ENOENT') {
        reject(new Error('Daemon socket not found. Is the daemon running?'));
      } else {
        reject(err);
      }
    });

    req.on('timeout', () => {
      req.destroy();
      reject(new Error('Request timed out'));
    });

    if (body) {
      req.write(JSON.stringify(body));
    }

    req.end();
  });
}

//@CLIENT_FACTORY
export function createClient(_serverName?: string): OrpheusClient {
  // For now, we just use the active server
  // TODO: Support _serverName parameter to switch servers
  const serverConfig = getActiveServer();

  return {
    async health(): Promise<HealthResponse> {
      return makeRequest<HealthResponse>('GET', '/v1/health', serverConfig, undefined, 5000);
    },

    async stats(agentName?: string): Promise<StatsResponse> {
      const path = agentName ? `/v1/stats?agent=${encodeURIComponent(agentName)}` : '/v1/stats';
      return makeRequest<StatsResponse>('GET', path, serverConfig);
    },

    async deploy(agentPath: string, options?: DeployOptions): Promise<DeployResponse> {
      // Validate agent path
      const validation = validateAgentPath(agentPath);
      if (!validation.valid) {
        throw new Error(validation.error || 'Invalid agent path');
      }

      const agentName = validation.agentName!;

      // Create tarball
      const tarball = await createTarball(agentPath);

      // Calculate checksum
      const checksum = calculateChecksum(tarball);

      // Upload to daemon with optional env vars
      return uploadAgent(serverConfig, agentName, tarball, checksum, options?.env);
    },

    async invoke(agentName: string, input: unknown): Promise<InvokeResponse> {
      return makeRequest<InvokeResponse>(
        'POST',
        `/v1/agents/${encodeURIComponent(agentName)}/run`,
        serverConfig,
        { input }
      );
    },

    async undeploy(agentName: string): Promise<{ success: boolean; message?: string }> {
      return makeRequest<{ success: boolean; message?: string }>(
        'DELETE',
        `/v1/agents/${encodeURIComponent(agentName)}`,
        serverConfig
      );
    },

    async list(): Promise<AgentListItem[]> {
      const response = await makeRequest<{ agents: AgentListItem[] }>(
        'GET',
        '/v1/agents',
        serverConfig
      );
      return response.agents || [];
    },

    async inspect(agentName: string): Promise<AgentDetails> {
      return makeRequest<AgentDetails>(
        'GET',
        `/v1/agents/${encodeURIComponent(agentName)}`,
        serverConfig
      );
    },

    async workspaceInfo(agentName: string): Promise<WorkspaceInfoResponse> {
      return makeRequest<WorkspaceInfoResponse>(
        'GET',
        `/v1/agents/${encodeURIComponent(agentName)}/workspace`,
        serverConfig
      );
    },

    async workspaceClean(agentName: string): Promise<WorkspaceCleanResponse> {
      return makeRequest<WorkspaceCleanResponse>(
        'DELETE',
        `/v1/agents/${encodeURIComponent(agentName)}/workspace`,
        serverConfig
      );
    },

    async getCrashedRequests(): Promise<CrashedRequestsResponse> {
      return makeRequest<CrashedRequestsResponse>(
        'GET',
        '/v1/execlog/crashed',
        serverConfig
      );
    },

    async getExecLogs(filters?: ExecLogFilters): Promise<ExecLogsResponse> {
      const params = new URLSearchParams();
      if (filters?.agent) params.append('agent', filters.agent);
      if (filters?.status) params.append('status', filters.status);
      if (filters?.session) params.append('session', filters.session);
      if (filters?.worker) params.append('worker', filters.worker);
      if (filters?.limit) params.append('limit', String(filters.limit));
      if (filters?.offset) params.append('offset', String(filters.offset));

      const query = params.toString();
      const path = `/v1/execlog${query ? '?' + query : ''}`;

      return makeRequest<ExecLogsResponse>('GET', path, serverConfig);
    },

    async getExecLogStats(agentName?: string): Promise<ExecLogStatsResponse> {
      const path = agentName
        ? `/v1/execlog/stats?agent=${encodeURIComponent(agentName)}`
        : '/v1/execlog/stats';
      return makeRequest<ExecLogStatsResponse>('GET', path, serverConfig);
    },

    close(): void {
      // no-op for HTTP
    },
  };
}

//@CONVENIENCE_FUNCTIONS
export async function testConnection(): Promise<boolean> {
  try {
    const client = createClient();
    await client.health();
    return true;
  } catch {
    return false;
  }
}

export async function getHealth(): Promise<HealthResponse | null> {
  try {
    const client = createClient();
    return await client.health();
  } catch {
    return null;
  }
}

export async function getStats(agentName?: string): Promise<StatsResponse | null> {
  try {
    const client = createClient();
    return await client.stats(agentName);
  } catch {
    return null;
  }
}
