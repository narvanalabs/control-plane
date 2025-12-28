// API client for Narvana control plane

const API_BASE = '/v1';

// Types
export interface App {
	id: string;
	owner_id: string;
	name: string;
	description?: string;
	services: ServiceConfig[];
	created_at: string;
	updated_at: string;
}

export type SourceType = 'git' | 'flake' | 'image';
export type ResourceTier = 'nano' | 'small' | 'medium' | 'large' | 'xlarge';
export type BuildStrategy = 'flake' | 'auto-go' | 'auto-rust' | 'auto-node' | 'auto-python' | 'dockerfile' | 'nixpacks' | 'auto';

export interface PortMapping {
	container_port: number;
	protocol?: string;
}

export interface HealthCheckConfig {
	path?: string;
	port?: number;
	interval_seconds?: number;
	timeout_seconds?: number;
	retries?: number;
}

export interface BuildConfig {
	entrypoint?: string;
	install_command?: string;
	build_command?: string;
	start_command?: string;
}

export interface ServiceConfig {
	name: string;
	source_type: SourceType;
	git_repo?: string;
	git_ref?: string;
	flake_output?: string;
	flake_uri?: string;
	image?: string;
	build_strategy?: BuildStrategy;
	build_config?: BuildConfig;
	resource_tier: ResourceTier;
	replicas: number;
	ports?: PortMapping[];
	health_check?: HealthCheckConfig;
	depends_on?: string[];
	env_vars?: Record<string, string>;
}

export type DeploymentStatus = 
	| 'pending'
	| 'building'
	| 'built'
	| 'scheduled'
	| 'starting'
	| 'running'
	| 'stopping'
	| 'stopped'
	| 'failed';

export interface Deployment {
	id: string;
	app_id: string;
	service_name: string;
	version: number;
	git_ref: string;
	git_commit?: string;
	build_type: 'oci' | 'pure-nix';
	artifact?: string;
	status: DeploymentStatus;
	node_id?: string;
	resource_tier: ResourceTier;
	depends_on?: string[];
	created_at: string;
	updated_at: string;
	started_at?: string;
	finished_at?: string;
}

export interface NodeResources {
	cpu_total: number;
	cpu_available: number;
	memory_total: number;
	memory_available: number;
	disk_total: number;
	disk_available: number;
}

export interface Node {
	id: string;
	hostname: string;
	address: string;
	grpc_port: number;
	healthy: boolean;
	resources?: NodeResources;
	cached_paths?: string[];
	last_heartbeat: string;
	registered_at: string;
}

export interface LogEntry {
	id: string;
	deployment_id: string;
	timestamp: string;
	level: string;
	message: string;
	source: string;
}

export interface User {
	id: string;
	email: string;
}

// API Error
export class APIError extends Error {
	constructor(
		public status: number,
		public code: string,
		message: string
	) {
		super(message);
		this.name = 'APIError';
	}
}

// Auth state
function getToken(): string | null {
	if (typeof localStorage === 'undefined') return null;
	return localStorage.getItem('narvana_token');
}

function setToken(token: string): void {
	localStorage.setItem('narvana_token', token);
}

function clearToken(): void {
	localStorage.removeItem('narvana_token');
	localStorage.removeItem('narvana_user');
}

function getUser(): User | null {
	if (typeof localStorage === 'undefined') return null;
	const user = localStorage.getItem('narvana_user');
	return user ? JSON.parse(user) : null;
}

function setUser(user: User): void {
	localStorage.setItem('narvana_user', JSON.stringify(user));
}

// HTTP helper
async function request<T>(
	method: string,
	path: string,
	body?: unknown,
	requireAuth = true
): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
	};

	if (requireAuth) {
		const token = getToken();
		if (!token) {
			throw new APIError(401, 'unauthorized', 'Not authenticated');
		}
		headers['Authorization'] = `Bearer ${token}`;
	}

	const response = await fetch(path, {
		method,
		headers,
		body: body ? JSON.stringify(body) : undefined,
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({ error: 'Unknown error' }));
		throw new APIError(response.status, error.code || 'error', error.error || error.message);
	}

	if (response.status === 204) {
		return undefined as T;
	}

	return response.json();
}

// Auth API
export const auth = {
	async checkSetup(): Promise<{ setup_complete: boolean }> {
		return request('GET', '/auth/setup', undefined, false);
	},

	async register(email: string, password: string): Promise<{ token: string; user_id: string; email: string }> {
		const result = await request<{ token: string; user_id: string; email: string }>(
			'POST',
			'/auth/register',
			{ email, password },
			false
		);
		setToken(result.token);
		setUser({ id: result.user_id, email: result.email });
		return result;
	},

	async login(email: string, password: string): Promise<{ token: string; user_id: string; email: string }> {
		const result = await request<{ token: string; user_id: string; email: string }>(
			'POST',
			'/auth/login',
			{ email, password },
			false
		);
		setToken(result.token);
		setUser({ id: result.user_id, email: result.email });
		return result;
	},

	async validate(): Promise<{ status: string; user_id: string }> {
		return request('GET', `${API_BASE}/auth/validate`);
	},

	logout(): void {
		clearToken();
	},

	isAuthenticated(): boolean {
		return !!getToken();
	},

	getUser,
	getToken,
};

// Apps API
export const apps = {
	async list(): Promise<App[]> {
		return request('GET', `${API_BASE}/apps`);
	},

	async get(appId: string): Promise<App> {
		return request('GET', `${API_BASE}/apps/${appId}`);
	},

	async create(name: string, description?: string): Promise<App> {
		return request('POST', `${API_BASE}/apps`, { name, description });
	},

	async delete(appId: string): Promise<void> {
		return request('DELETE', `${API_BASE}/apps/${appId}`);
	},
};

// Services API
export const services = {
	async list(appId: string): Promise<ServiceConfig[]> {
		return request('GET', `${API_BASE}/apps/${appId}/services`);
	},

	async get(appId: string, serviceName: string): Promise<ServiceConfig> {
		return request('GET', `${API_BASE}/apps/${appId}/services/${serviceName}`);
	},

	async create(appId: string, service: Partial<ServiceConfig>): Promise<ServiceConfig> {
		return request('POST', `${API_BASE}/apps/${appId}/services`, service);
	},

	async update(appId: string, serviceName: string, updates: Partial<ServiceConfig>): Promise<ServiceConfig> {
		return request('PATCH', `${API_BASE}/apps/${appId}/services/${serviceName}`, updates);
	},

	async delete(appId: string, serviceName: string): Promise<void> {
		return request('DELETE', `${API_BASE}/apps/${appId}/services/${serviceName}`);
	},

	async deploy(appId: string, serviceName: string, gitRef?: string): Promise<Deployment> {
		return request('POST', `${API_BASE}/apps/${appId}/services/${serviceName}/deploy`, { git_ref: gitRef });
	},
};

// Deployments API
export const deployments = {
	async list(appId: string): Promise<Deployment[]> {
		return request('GET', `${API_BASE}/apps/${appId}/deployments`);
	},

	async get(deploymentId: string): Promise<Deployment> {
		return request('GET', `${API_BASE}/deployments/${deploymentId}`);
	},

	async create(appId: string, gitRef?: string, serviceName?: string): Promise<Deployment | Deployment[]> {
		return request('POST', `${API_BASE}/apps/${appId}/deploy`, { git_ref: gitRef, service_name: serviceName });
	},

	async rollback(deploymentId: string): Promise<Deployment> {
		return request('POST', `${API_BASE}/deployments/${deploymentId}/rollback`);
	},
};

// Nodes API
export const nodes = {
	async list(): Promise<Node[]> {
		return request('GET', `${API_BASE}/nodes`);
	},
};

// Secrets API
export const secrets = {
	async list(appId: string): Promise<{ keys: string[] }> {
		return request('GET', `${API_BASE}/apps/${appId}/secrets`);
	},

	async create(appId: string, key: string, value: string): Promise<{ key: string; status: string }> {
		return request('POST', `${API_BASE}/apps/${appId}/secrets`, { key, value });
	},

	async delete(appId: string, key: string): Promise<void> {
		return request('DELETE', `${API_BASE}/apps/${appId}/secrets/${key}`);
	},
};

// Logs API
export const logs = {
	async get(
		appId: string,
		options?: { limit?: number; source?: string; deployment_id?: string }
	): Promise<{ deployment_id: string; logs: LogEntry[] }> {
		const params = new URLSearchParams();
		if (options?.limit) params.set('limit', String(options.limit));
		if (options?.source) params.set('source', options.source);
		if (options?.deployment_id) params.set('deployment_id', options.deployment_id);
		const query = params.toString();
		return request('GET', `${API_BASE}/apps/${appId}/logs${query ? `?${query}` : ''}`);
	},

	/**
	 * Get the URL for SSE log streaming with authentication token.
	 * Returns null if not authenticated.
	 */
	getStreamUrl(
		appId: string,
		options?: { source?: string; deployment_id?: string }
	): string | null {
		const token = getToken();
		if (!token) return null;
		
		const params = new URLSearchParams();
		params.set('token', token);
		if (options?.source && options.source !== 'all') {
			params.set('source', options.source);
		}
		if (options?.deployment_id) {
			params.set('deployment_id', options.deployment_id);
		}
		
		return `${API_BASE}/apps/${appId}/logs/stream?${params.toString()}`;
	},
};

// Detection API
export interface DetectResponse {
	strategy: BuildStrategy;
	framework: string;
	version?: string;
	suggested_config?: Record<string, unknown>;
	recommended_build_type: 'oci' | 'pure-nix';
	entry_points?: string[];
	confidence: number;
	warnings?: string[];
}

export interface DetectErrorResponse {
	error: string;
	code: string;
	suggestions?: string[];
}

export const detect = {
	async analyze(gitUrl: string, gitRef?: string): Promise<DetectResponse> {
		return request('POST', `${API_BASE}/detect`, { git_url: gitUrl, git_ref: gitRef });
	},
};

// Preview API
export interface PreviewResponse {
	generated_flake?: string;
	strategy: BuildStrategy;
	build_type: 'oci' | 'pure-nix';
	estimated_build_time: number;
	estimated_resources: {
		memory_mb: number;
		disk_mb: number;
		cpu_cores: number;
	};
	detection?: {
		framework: string;
		version?: string;
		entry_point?: string;
		confidence: number;
	};
	warnings?: string[];
	flake_valid?: boolean;
	validation_error?: string;
}

export const preview = {
	async generate(
		appId: string,
		serviceName: string,
		buildStrategy?: BuildStrategy,
		buildConfig?: BuildConfig
	): Promise<PreviewResponse> {
		return request('POST', `${API_BASE}/apps/${appId}/services/${serviceName}/preview`, {
			build_strategy: buildStrategy,
			build_config: buildConfig,
		});
	},
};


