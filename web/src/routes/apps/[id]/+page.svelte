<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { 
		apps, services, deployments, secrets, logs, detect, preview,
		type App, type ServiceConfig, type Deployment, type LogEntry,
		type BuildStrategy, type DetectResponse, type PreviewResponse,
		type ResourceTier, APIError
	} from '$lib/api';
	import { Card, Button, Input, StatusBadge, Dialog, Tabs, EmptyState, PageHeader } from '$lib/components';
	import { ServiceCard } from '$lib/components/domain';
	import { formatRelativeTime } from '$lib/utils/formatters';
	import { MoreVertical, Trash2, Settings, Play, Plus, Key, FileText, Server, History, Lock, Cog } from 'lucide-svelte';

	// State
	let app = $state<App | null>(null);
	let deploymentList = $state<Deployment[]>([]);
	let secretKeys = $state<string[]>([]);
	let logEntries = $state<LogEntry[]>([]);
	let loading = $state(true);
	let activeTab = $state<string>('environment');
	let error = $state('');

	// Domain configuration state
	let domainPorts = $state<Record<string, number>>({});
	let savingDomain = $state<string | null>(null);

	// Service modal
	let showServiceModal = $state(false);
	let editingServiceName = $state<string | null>(null);
	let serviceForm = $state({
		name: '',
		git_repo: '',
		git_ref: 'main',
		build_strategy: 'auto' as BuildStrategy,
		resource_tier: 'small' as ResourceTier,
		replicas: 1,
		port: 8080,
	});
	let initialServiceForm = $state({
		name: '',
		git_repo: '',
		git_ref: 'main',
		build_strategy: 'auto' as BuildStrategy,
		resource_tier: 'small' as ResourceTier,
		replicas: 1,
		port: 8080,
	});
	let creatingService = $state(false);
	let serviceError = $state('');
	let detecting = $state(false);
	let detectResult = $state<DetectResponse | null>(null);

	// Preview modal
	let showPreviewModal = $state(false);
	let previewServiceName = $state('');
	let previewLoading = $state(false);
	let previewResult = $state<PreviewResponse | null>(null);
	let previewError = $state('');

	// Secret modal
	let showSecretModal = $state(false);
	let secretKey = $state('');
	let secretValue = $state('');
	let creatingSecret = $state(false);

	// Deploy modal
	let showDeployModal = $state(false);
	let deployServiceName = $state('');
	let deployGitRef = $state('');
	let deploying = $state(false);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTarget = $state<{ type: 'app' | 'service' | 'secret'; name: string } | null>(null);
	let deleting = $state(false);

	// Settings dropdown
	let showSettingsMenu = $state(false);

	// Log filtering & streaming
	let logSource = $state<'all' | 'build' | 'runtime'>('all');
	let selectedDeploymentId = $state('');
	let logStreaming = $state(false);
	let logEventSource: EventSource | null = null;
	let autoScroll = $state(true);
	let logContainer = $state<HTMLElement | null>(null);

	// Build strategy options
	const buildStrategies: { value: BuildStrategy; label: string; description: string }[] = [
		{ value: 'auto', label: 'Auto Detect', description: 'Automatically detect language and framework' },
		{ value: 'flake', label: 'Nix Flake', description: 'Use existing flake.nix in repository' },
		{ value: 'auto-go', label: 'Go', description: 'Auto-generate Nix flake for Go projects' },
		{ value: 'auto-node', label: 'Node.js', description: 'Auto-generate Nix flake for Node.js projects' },
		{ value: 'auto-python', label: 'Python', description: 'Auto-generate Nix flake for Python projects' },
		{ value: 'auto-rust', label: 'Rust', description: 'Auto-generate Nix flake for Rust projects' },
		{ value: 'dockerfile', label: 'Dockerfile', description: 'Build using Dockerfile in repository' },
		{ value: 'nixpacks', label: 'Nixpacks', description: 'Use Nixpacks for automatic containerization' },
	];

	// Resource tier options
	const resourceTiers: { value: ResourceTier; label: string; description: string }[] = [
		{ value: 'nano', label: 'Nano', description: '256MB RAM' },
		{ value: 'small', label: 'Small', description: '512MB RAM' },
		{ value: 'medium', label: 'Medium', description: '1GB RAM' },
		{ value: 'large', label: 'Large', description: '2GB RAM' },
		{ value: 'xlarge', label: 'XLarge', description: '4GB RAM' },
	];

	const appId = $derived($page.params.id as string);

	// Tab definitions
	const tabs = $derived([
		{ value: 'environment', label: 'Environment' },
		{ value: 'deployments', label: 'Deployments' },
		{ value: 'logs', label: 'Logs' },
		{ value: 'secrets', label: 'Secrets' },
		{ value: 'settings', label: 'Settings' },
	]);

	// Check if form is dirty (has unsaved changes)
	const isFormDirty = $derived(() => {
		if (!editingServiceName) return false;
		return (
			serviceForm.name !== initialServiceForm.name ||
			serviceForm.git_repo !== initialServiceForm.git_repo ||
			serviceForm.git_ref !== initialServiceForm.git_ref ||
			serviceForm.build_strategy !== initialServiceForm.build_strategy ||
			serviceForm.resource_tier !== initialServiceForm.resource_tier ||
			serviceForm.replicas !== initialServiceForm.replicas ||
			serviceForm.port !== initialServiceForm.port
		);
	});

	// Sync active tab with URL
	$effect(() => {
		const urlTab = $page.url.searchParams.get('tab');
		if (urlTab && tabs.some(t => t.value === urlTab)) {
			activeTab = urlTab;
		}
	});

	function handleTabChange(tabId: string) {
		activeTab = tabId;
		const url = new URL($page.url);
		url.searchParams.set('tab', tabId);
		goto(url.toString(), { replaceState: true, noScroll: true });
	}

	// Auto-detect build strategy
	async function runDetection() {
		if (!serviceForm.git_repo) {
			serviceError = 'Enter a git repository URL first';
			return;
		}
		detecting = true;
		detectResult = null;
		serviceError = '';
		try {
			const result = await detect.analyze(serviceForm.git_repo, serviceForm.git_ref);
			detectResult = result;
			if (result.strategy) {
				serviceForm.build_strategy = result.strategy;
			}
		} catch (err) {
			if (err instanceof APIError) {
				serviceError = err.message;
			} else {
				serviceError = 'Detection failed';
			}
		} finally {
			detecting = false;
		}
	}

	// Preview build for a service
	async function runPreview(serviceName: string) {
		previewServiceName = serviceName;
		previewLoading = true;
		previewResult = null;
		previewError = '';
		showPreviewModal = true;
		try {
			previewResult = await preview.generate(appId, serviceName);
		} catch (err) {
			if (err instanceof APIError) {
				previewError = err.message;
			} else {
				previewError = 'Preview generation failed';
			}
		} finally {
			previewLoading = false;
		}
	}

	// Load existing logs and start streaming automatically
	async function loadAndStreamLogs() {
		try {
			const logsData = await logs.get(appId, {
				limit: 500,
				source: logSource === 'all' ? undefined : logSource,
				deployment_id: selectedDeploymentId || undefined,
			});
			logEntries = (logsData.logs || []).reverse();
			
			requestAnimationFrame(() => {
				if (logContainer) {
					logContainer.scrollTop = logContainer.scrollHeight;
				}
			});
		} catch (err) {
			console.error('Failed to load logs:', err);
		}
		
		startLogStream();
	}

	// Start real-time log streaming
	function startLogStream() {
		stopLogStream();
		
		const url = logs.getStreamUrl(appId, {
			source: logSource === 'all' ? undefined : logSource,
			deployment_id: selectedDeploymentId || undefined,
		});
		
		if (!url) {
			console.error('Cannot start log stream: not authenticated');
			return;
		}
		
		logEventSource = new EventSource(url);
		logStreaming = true;
		
		const seenLogIds = new Set(logEntries.map(l => l.id));
		
		logEventSource.addEventListener('connected', (e) => {
			const data = JSON.parse((e as MessageEvent).data);
			console.log('Log stream connected:', data);
		});
		
		logEventSource.addEventListener('log', (e) => {
			const log = JSON.parse((e as MessageEvent).data);
			
			if (seenLogIds.has(log.id)) return;
			seenLogIds.add(log.id);
			
			logEntries = [...logEntries, log];
			
			if (autoScroll && logContainer) {
				requestAnimationFrame(() => {
					if (logContainer) {
						logContainer.scrollTop = logContainer.scrollHeight;
					}
				});
			}
		});
		
		logEventSource.addEventListener('complete', (e) => {
			const data = JSON.parse((e as MessageEvent).data);
			console.log('Deployment complete:', data);
		});
		
		logEventSource.onerror = (err) => {
			console.error('Log stream error:', err);
		};
	}

	function stopLogStream() {
		if (logEventSource) {
			logEventSource.close();
			logEventSource = null;
		}
		logStreaming = false;
	}

	$effect(() => {
		return () => {
			stopLogStream();
		};
	});

	$effect(() => {
		if (activeTab === 'logs' && appId && !logStreaming) {
			loadAndStreamLogs();
		} else if (activeTab !== 'logs' && logStreaming) {
			stopLogStream();
		}
	});

	$effect(() => {
		if (appId) loadAppData();
	});

	async function loadAppData() {
		loading = true;
		error = '';
		try {
			const [appData, deploymentsData, secretsData] = await Promise.all([
				apps.get(appId),
				deployments.list(appId).catch(() => []),
				secrets.list(appId).catch(() => ({ keys: [] })),
			]);
			app = appData;
			deploymentList = deploymentsData.sort((a, b) => 
				new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
			);
			secretKeys = secretsData.keys;

			if (deploymentsData.length > 0) {
				const logsData = await logs.get(appId, { limit: 100 }).catch(() => ({ logs: [] }));
				logEntries = logsData.logs;
			}
		} catch (err) {
			if (err instanceof APIError && err.status === 404) {
				error = 'Application not found';
			} else {
				error = 'Failed to load application';
			}
		} finally {
			loading = false;
		}
	}

	function openAddServiceModal() {
		editingServiceName = null;
		serviceForm = {
			name: '',
			git_repo: '',
			git_ref: 'main',
			build_strategy: 'auto',
			resource_tier: 'small',
			replicas: 1,
			port: 8080,
		};
		initialServiceForm = { ...serviceForm };
		detectResult = null;
		serviceError = '';
		showServiceModal = true;
	}

	function openEditServiceModal(service: ServiceConfig) {
		editingServiceName = service.name;
		serviceForm = {
			name: service.name,
			git_repo: service.git_repo || '',
			git_ref: service.git_ref || 'main',
			build_strategy: service.build_strategy || 'auto',
			resource_tier: service.resource_tier,
			replicas: service.replicas,
			port: service.ports?.[0]?.container_port ?? 8080,
		};
		initialServiceForm = { ...serviceForm };
		detectResult = null;
		serviceError = '';
		showServiceModal = true;
	}

	async function saveService(e: Event) {
		e.preventDefault();
		serviceError = '';
		creatingService = true;
		try {
			if (editingServiceName) {
				await services.update(appId, editingServiceName, {
					resource_tier: serviceForm.resource_tier,
					replicas: serviceForm.replicas,
					ports: serviceForm.port > 0 ? [{ container_port: serviceForm.port, protocol: 'tcp' }] : [],
				});
			} else {
				await services.create(appId, {
					name: serviceForm.name,
					git_repo: serviceForm.git_repo,
					git_ref: serviceForm.git_ref,
					build_strategy: serviceForm.build_strategy,
					resource_tier: serviceForm.resource_tier,
					replicas: serviceForm.replicas,
					ports: serviceForm.port > 0 ? [{ container_port: serviceForm.port, protocol: 'tcp' }] : [],
				});
			}
			showServiceModal = false;
			await loadAppData();
		} catch (err) {
			serviceError = err instanceof Error ? err.message : 'Failed to save service';
		} finally {
			creatingService = false;
		}
	}

	async function createSecret(e: Event) {
		e.preventDefault();
		creatingSecret = true;
		try {
			await secrets.create(appId, secretKey, secretValue);
			showSecretModal = false;
			secretKey = '';
			secretValue = '';
			await loadAppData();
		} catch (err) {
			console.error('Failed to create secret:', err);
		} finally {
			creatingSecret = false;
		}
	}

	async function deployApp(e: Event) {
		e.preventDefault();
		deploying = true;
		try {
			await deployments.create(appId, deployGitRef || undefined, deployServiceName || undefined);
			showDeployModal = false;
			deployServiceName = '';
			deployGitRef = '';
			await loadAppData();
		} catch (err) {
			console.error('Failed to deploy:', err);
		} finally {
			deploying = false;
		}
	}

	async function handleDelete() {
		if (!deleteTarget) return;
		deleting = true;
		try {
			if (deleteTarget.type === 'app') {
				await apps.delete(appId);
				goto('/apps');
			} else if (deleteTarget.type === 'service') {
				await services.delete(appId, deleteTarget.name);
				await loadAppData();
			} else if (deleteTarget.type === 'secret') {
				await secrets.delete(appId, deleteTarget.name);
				await loadAppData();
			}
			showDeleteModal = false;
			deleteTarget = null;
		} catch (err) {
			console.error('Delete failed:', err);
		} finally {
			deleting = false;
		}
	}

	function formatDate(date: string): string {
		return new Date(date).toLocaleString('en-US', {
			month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
		});
	}

	function getServiceStatus(serviceName: string): Deployment | undefined {
		return deploymentList.find(d => d.service_name === serviceName);
	}

	// Initialize domain ports from services when app loads
	$effect(() => {
		if (app?.services) {
			const ports: Record<string, number> = {};
			for (const svc of app.services) {
				ports[svc.name] = svc.ports?.[0]?.container_port ?? 8080;
			}
			domainPorts = ports;
		}
	});

	async function saveDomainConfig(serviceName: string) {
		savingDomain = serviceName;
		try {
			const port = domainPorts[serviceName];
			await services.update(appId, serviceName, {
				ports: port > 0 ? [{ container_port: port, protocol: 'tcp' }] : [],
			});
			await loadAppData();
		} catch (err) {
			console.error('Failed to save domain config:', err);
		} finally {
			savingDomain = null;
		}
	}

	function getWildcardDomain(serviceName: string): string {
		const appName = app?.name?.toLowerCase().replace(/[^a-z0-9-]/g, '-') ?? 'app';
		const svcName = serviceName.toLowerCase().replace(/[^a-z0-9-]/g, '-');
		return `${appName}-${svcName}.narvana.local:8088`;
	}
</script>

<svelte:window onclick={() => showSettingsMenu && (showSettingsMenu = false)} />

<div class="min-h-screen bg-[var(--color-background)]">
	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="w-8 h-8 border-2 border-[var(--color-primary)] border-t-transparent rounded-full animate-spin"></div>
		</div>
	{:else if error}
		<div class="p-8">
			<Card class="p-12 text-center">
				<p class="text-[var(--color-error)] mb-4">{error}</p>
				<Button onclick={() => goto('/apps')}>Back to Apps</Button>
			</Card>
		</div>
	{:else if app}
		<!-- Breadcrumb Navigation -->
		<div class="border-b border-[var(--color-border)] bg-[var(--color-surface)]">
			<div class="px-6 py-3">
				<nav class="flex items-center gap-2 text-sm" aria-label="Breadcrumb">
					<a href="/apps" class="text-[var(--color-text-muted)] hover:text-[var(--color-text)] transition-colors">
						Applications
					</a>
					<span class="text-[var(--color-text-muted)]">/</span>
					<span class="font-medium text-[var(--color-text)]">{app.name}</span>
				</nav>
			</div>
		</div>

		<!-- Page Header -->
		<PageHeader 
			title={app.name} 
			description={app.description || 'No description'}
			tabs={tabs}
			activeTab={activeTab}
			onTabChange={handleTabChange}
		>
			{#snippet actions()}
				<Button 
					variant="primary"
					onclick={() => { showDeployModal = true; }}
				>
					<Play class="w-4 h-4" />
					Deploy
				</Button>
				
				<!-- Settings dropdown -->
				<div class="relative">
					<Button 
						variant="outline"
						onclick={(e) => { e.stopPropagation(); showSettingsMenu = !showSettingsMenu; }}
					>
						<MoreVertical class="w-4 h-4" />
					</Button>
					
					{#if showSettingsMenu}
						<div 
							class="absolute right-0 top-full mt-1 w-48 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-lg)] shadow-[var(--shadow-lg)] py-1 z-50"
							onclick={(e) => e.stopPropagation()}
						>
							<button
								onclick={() => { handleTabChange('settings'); showSettingsMenu = false; }}
								class="w-full flex items-center gap-2 px-3 py-2 text-sm text-[var(--color-text-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-surface-hover)] transition-colors"
							>
								<Settings class="w-4 h-4" />
								Settings
							</button>
							<div class="border-t border-[var(--color-border)] my-1"></div>
							<button
								onclick={() => { deleteTarget = { type: 'app', name: app!.name }; showDeleteModal = true; showSettingsMenu = false; }}
								class="w-full flex items-center gap-2 px-3 py-2 text-sm text-[var(--color-error)] hover:bg-[var(--color-error-light)] transition-colors"
							>
								<Trash2 class="w-4 h-4" />
								Delete Application
							</button>
						</div>
					{/if}
				</div>
			{/snippet}
		</PageHeader>

		<!-- Tab Content -->
		<div class="p-6">
			{#if activeTab === 'environment'}
				<!-- Environment Tab - Services -->
				<div class="flex justify-between items-center mb-6">
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-text)]">Services</h2>
						<p class="text-sm text-[var(--color-text-secondary)]">Manage the services in your application</p>
					</div>
					<Button size="sm" onclick={openAddServiceModal}>
						<Plus class="w-4 h-4" />
						Add Service
					</Button>
				</div>

				{#if (app.services?.length ?? 0) === 0}
					<EmptyState
						icon={Server}
						title="No services defined"
						description="Add your first service to start deploying your application."
					>
						{#snippet action()}
							<Button onclick={openAddServiceModal}>
								<Plus class="w-4 h-4" />
								Add First Service
							</Button>
						{/snippet}
					</EmptyState>
				{:else}
					<div class="grid gap-4">
						{#each app.services ?? [] as service}
							{@const deployment = getServiceStatus(service.name)}
							<ServiceCard
								{service}
								{deployment}
								onDeploy={() => { deployServiceName = service.name; showDeployModal = true; }}
								onEdit={() => openEditServiceModal(service)}
								onPreview={() => runPreview(service.name)}
								onDelete={() => { deleteTarget = { type: 'service', name: service.name }; showDeleteModal = true; }}
							/>
						{/each}
					</div>
				{/if}

			{:else if activeTab === 'deployments'}
				<!-- Deployments Tab -->
				<div class="mb-6">
					<h2 class="text-lg font-semibold text-[var(--color-text)]">Deployment History</h2>
					<p class="text-sm text-[var(--color-text-secondary)]">View and manage your deployment history</p>
				</div>

				{#if deploymentList.length === 0}
					<EmptyState
						icon={History}
						title="No deployments yet"
						description="Deploy your first service to see deployment history here."
					>
						{#snippet action()}
							<Button onclick={() => showDeployModal = true}>
								<Play class="w-4 h-4" />
								Deploy Now
							</Button>
						{/snippet}
					</EmptyState>
				{:else}
					<div class="space-y-3">
						{#each deploymentList as deployment}
							<Card padding="md">
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-4">
										<StatusBadge status={deployment.status} />
										<div>
											<p class="font-medium text-[var(--color-text)]">
												{deployment.service_name}
												<span class="text-[var(--color-text-muted)] font-normal">
													#{deployment.version}
												</span>
											</p>
											<p class="text-sm text-[var(--color-text-secondary)]">
												{formatDate(deployment.created_at)}
												{#if deployment.git_ref}
													• <span class="font-mono">{deployment.git_ref}</span>
												{/if}
												{#if deployment.git_commit}
													• <span class="font-mono text-xs">{deployment.git_commit.slice(0, 7)}</span>
												{/if}
											</p>
										</div>
									</div>
									<div class="text-sm text-[var(--color-text-muted)]">
										{formatRelativeTime(deployment.created_at)}
									</div>
								</div>
							</Card>
						{/each}
					</div>
				{/if}

			{:else if activeTab === 'logs'}
				<!-- Logs Tab -->
				<div class="flex items-center justify-between mb-6">
					<div class="flex items-center gap-3">
						<div>
							<h2 class="text-lg font-semibold text-[var(--color-text)]">Build & Runtime Logs</h2>
							<p class="text-sm text-[var(--color-text-secondary)]">View logs from your deployments</p>
						</div>
						{#if logStreaming}
							<span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-[var(--color-success-light)] text-[var(--color-success)] text-xs font-medium">
								<span class="w-2 h-2 rounded-full bg-[var(--color-success)] animate-pulse"></span>
								Live
							</span>
						{/if}
					</div>
					<div class="flex gap-2 items-center">
						<select
							bind:value={logSource}
							onchange={() => loadAndStreamLogs()}
							class="px-3 py-1.5 text-sm rounded-[var(--radius-md)] bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)]"
						>
							<option value="all">All Sources</option>
							<option value="build">Build Logs</option>
							<option value="runtime">Runtime Logs</option>
						</select>

						{#if deploymentList.length > 0}
							<select
								bind:value={selectedDeploymentId}
								onchange={() => loadAndStreamLogs()}
								class="px-3 py-1.5 text-sm rounded-[var(--radius-md)] bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)]"
							>
								<option value="">Latest Deployment</option>
								{#each deploymentList.slice(0, 10) as dep}
									<option value={dep.id}>
										{dep.service_name} #{dep.version} ({dep.status})
									</option>
								{/each}
							</select>
						{/if}

						<label class="flex items-center gap-1.5 text-sm text-[var(--color-text-muted)] cursor-pointer">
							<input 
								type="checkbox" 
								bind:checked={autoScroll}
								class="w-4 h-4 rounded accent-[var(--color-primary)]"
							/>
							Auto-scroll
						</label>
					</div>
				</div>

				{#if logEntries.length === 0}
					<Card class="p-8 text-center bg-[#0d1117]">
						{#if logStreaming}
							<div class="flex items-center justify-center gap-2 text-[var(--color-text-muted)]">
								<span class="w-2 h-2 rounded-full bg-[var(--color-success)] animate-pulse"></span>
								Waiting for logs...
							</div>
							<p class="text-sm text-[var(--color-text-muted)] mt-2">
								Deploy a service to see build output here in real-time.
							</p>
						{:else}
							<p class="text-[var(--color-text-muted)]">No logs available</p>
							<p class="text-sm text-[var(--color-text-muted)] mt-2">
								Deploy a service to see build and runtime logs here.
							</p>
						{/if}
					</Card>
				{:else}
					<Card class="p-0 overflow-hidden bg-[#0d1117]">
						<div 
							bind:this={logContainer}
							class="h-[600px] overflow-y-auto font-mono text-sm p-4 scroll-smooth"
						>
							{#each logEntries as log, i}
								<div 
									class="flex py-0.5 hover:bg-white/5 group
										{log.level === 'error' ? 'bg-red-500/10' : ''}"
								>
									<span class="w-12 flex-shrink-0 text-right pr-4 text-gray-600 select-none">
										{i + 1}
									</span>
									<span class="w-24 flex-shrink-0 text-gray-500">
										{new Date(log.timestamp).toLocaleTimeString('en-US', { hour12: false })}
									</span>
									<span class="w-12 flex-shrink-0 font-semibold
										{log.level === 'error' ? 'text-red-400' : 
										 log.level === 'warn' ? 'text-yellow-400' : 
										 log.level === 'info' ? 'text-cyan-400' : 
										 log.level === 'debug' ? 'text-purple-400' :
										 'text-gray-400'}">
										{log.level.toUpperCase().slice(0, 4)}
									</span>
									<span class="w-16 flex-shrink-0">
										<span class="inline-block px-1.5 py-0.5 rounded text-xs
											{log.source === 'build' ? 'bg-blue-500/20 text-blue-400' : 'bg-purple-500/20 text-purple-400'}">
											{log.source || 'sys'}
										</span>
									</span>
									<span class="flex-1 text-gray-200 whitespace-pre-wrap break-all">
										{log.message}
									</span>
								</div>
							{/each}
							{#if logStreaming}
								<div class="flex items-center gap-2 text-gray-500 mt-2 pt-2 border-t border-white/10">
									<span class="w-2 h-2 rounded-full bg-green-400 animate-pulse"></span>
									Streaming...
								</div>
							{/if}
						</div>
					</Card>
					<div class="flex items-center justify-between mt-2">
						<p class="text-sm text-[var(--color-text-muted)]">
							{logEntries.length} log {logEntries.length === 1 ? 'entry' : 'entries'}
							{#if logStreaming}
								• Live streaming active
							{/if}
						</p>
						{#if logEntries.length > 0}
							<button 
								onclick={() => { if (logContainer) logContainer.scrollTop = logContainer.scrollHeight; }}
								class="text-sm text-[var(--color-primary)] hover:underline"
							>
								↓ Jump to bottom
							</button>
						{/if}
					</div>
				{/if}

			{:else if activeTab === 'secrets'}
				<!-- Secrets Tab -->
				<div class="flex justify-between items-center mb-6">
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-text)]">Environment Secrets</h2>
						<p class="text-sm text-[var(--color-text-secondary)]">Manage environment variables for your services</p>
					</div>
					<Button size="sm" onclick={() => showSecretModal = true}>
						<Plus class="w-4 h-4" />
						Add Secret
					</Button>
				</div>

				{#if secretKeys.length === 0}
					<EmptyState
						icon={Lock}
						title="No secrets defined"
						description="Add environment secrets to configure your services securely."
					>
						{#snippet action()}
							<Button onclick={() => showSecretModal = true}>
								<Plus class="w-4 h-4" />
								Add First Secret
							</Button>
						{/snippet}
					</EmptyState>
				{:else}
					<div class="space-y-2">
						{#each secretKeys as key}
							<Card padding="md" class="flex items-center justify-between">
								<div class="flex items-center gap-3">
									<Key class="w-4 h-4 text-[var(--color-warning)]" />
									<code class="font-mono text-sm text-[var(--color-text)]">{key}</code>
								</div>
								<Button 
									size="sm" 
									variant="ghost"
									onclick={() => { deleteTarget = { type: 'secret', name: key }; showDeleteModal = true; }}
								>
									<Trash2 class="w-4 h-4" />
								</Button>
							</Card>
						{/each}
					</div>
				{/if}

			{:else if activeTab === 'settings'}
				<!-- Settings Tab -->
				<div class="mb-6">
					<h2 class="text-lg font-semibold text-[var(--color-text)]">Application Settings</h2>
					<p class="text-sm text-[var(--color-text-secondary)]">Configure your application settings</p>
				</div>

				<Card padding="lg">
					<div class="space-y-6">
						<div>
							<label class="block text-sm font-medium text-[var(--color-text)] mb-1">Application Name</label>
							<p class="text-[var(--color-text-secondary)]">{app.name}</p>
						</div>
						<div>
							<label class="block text-sm font-medium text-[var(--color-text)] mb-1">Description</label>
							<p class="text-[var(--color-text-secondary)]">{app.description || 'No description'}</p>
						</div>
						<div>
							<label class="block text-sm font-medium text-[var(--color-text)] mb-1">Created</label>
							<p class="text-[var(--color-text-secondary)]">{formatDate(app.created_at)}</p>
						</div>
						<div>
							<label class="block text-sm font-medium text-[var(--color-text)] mb-1">Last Updated</label>
							<p class="text-[var(--color-text-secondary)]">{formatDate(app.updated_at)}</p>
						</div>
					</div>
				</Card>

				<Card padding="lg" class="mt-6 border-[var(--color-error)]/30">
					<h3 class="text-lg font-semibold text-[var(--color-error)] mb-2">Danger Zone</h3>
					<p class="text-sm text-[var(--color-text-secondary)] mb-4">
						Once you delete an application, there is no going back. Please be certain.
					</p>
					<Button 
						variant="danger"
						onclick={() => { deleteTarget = { type: 'app', name: app!.name }; showDeleteModal = true; }}
					>
						<Trash2 class="w-4 h-4" />
						Delete Application
					</Button>
				</Card>
			{/if}
		</div>
	{/if}
</div>


<!-- Add/Edit Service Modal -->
<Dialog
	bind:open={showServiceModal}
	title={editingServiceName ? `Edit Service: ${editingServiceName}` : 'Add Service'}
	description={editingServiceName ? 'Update service configuration' : 'Configure a new service for your application'}
>
	<form id="service-form" onsubmit={saveService} class="space-y-6">
		<!-- Source Section -->
		<div class="space-y-4">
			<h3 class="text-sm font-semibold text-[var(--color-text)] uppercase tracking-wide">Source</h3>
			
			{#if !editingServiceName}
				<Input label="Service Name" placeholder="api" bind:value={serviceForm.name} required />
				
				<div class="space-y-1.5">
					<label for="git-repo" class="block text-sm font-medium text-[var(--color-text-secondary)]">
						Git Repository <span class="text-[var(--color-error)]">*</span>
					</label>
					<div class="flex gap-2">
						<input
							id="git-repo"
							type="text"
							placeholder="github.com/org/repo"
							bind:value={serviceForm.git_repo}
							required
							class="flex-1 px-4 py-2.5 rounded-[var(--radius-md)] bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)] focus:outline-none focus:border-[var(--color-primary)]"
						/>
						<Button 
							type="button" 
							variant="secondary" 
							onclick={runDetection}
							loading={detecting}
							disabled={!serviceForm.git_repo}
						>
							Detect
						</Button>
					</div>
				</div>

				<Input label="Git Ref" placeholder="main" bind:value={serviceForm.git_ref} />
			{/if}

			{#if detectResult}
				<div class="p-4 rounded-[var(--radius-lg)] bg-[var(--color-success-light)] border border-[var(--color-success)]/30">
					<div class="flex items-center gap-2 mb-2">
						<span class="text-[var(--color-success)]">✓</span>
						<span class="font-medium text-[var(--color-text)]">Detection Result</span>
						<span class="text-sm text-[var(--color-text-muted)]">
							({Math.round(detectResult.confidence * 100)}% confidence)
						</span>
					</div>
					<div class="grid grid-cols-2 gap-2 text-sm">
						<div>
							<span class="text-[var(--color-text-muted)]">Framework:</span>
							<span class="ml-1 font-medium text-[var(--color-text)]">{detectResult.framework || 'Unknown'}</span>
						</div>
						<div>
							<span class="text-[var(--color-text-muted)]">Strategy:</span>
							<span class="ml-1 font-medium text-[var(--color-text)]">{detectResult.strategy}</span>
						</div>
					</div>
				</div>
			{/if}
		</div>

		<!-- Build Section -->
		<div class="space-y-4">
			<h3 class="text-sm font-semibold text-[var(--color-text)] uppercase tracking-wide">Build</h3>
			
			{#if !editingServiceName}
				<div>
					<label for="build-strategy" class="block text-sm font-medium text-[var(--color-text-secondary)] mb-1.5">
						Build Strategy
					</label>
					<select 
						id="build-strategy"
						bind:value={serviceForm.build_strategy}
						class="w-full px-4 py-2.5 rounded-[var(--radius-md)] bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)]"
					>
						{#each buildStrategies as strategy}
							<option value={strategy.value}>{strategy.label} - {strategy.description}</option>
						{/each}
					</select>
				</div>
			{/if}
		</div>

		<!-- Resources Section -->
		<div class="space-y-4">
			<h3 class="text-sm font-semibold text-[var(--color-text)] uppercase tracking-wide">Resources</h3>
			
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="resource-tier" class="block text-sm font-medium text-[var(--color-text-secondary)] mb-1.5">
						Resource Tier
					</label>
					<select 
						id="resource-tier"
						bind:value={serviceForm.resource_tier}
						class="w-full px-4 py-2.5 rounded-[var(--radius-md)] bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)]"
					>
						{#each resourceTiers as tier}
							<option value={tier.value}>{tier.label} ({tier.description})</option>
						{/each}
					</select>
				</div>
				<Input 
					type="number" 
					label="Replicas" 
					bind:value={serviceForm.replicas}
				/>
			</div>
		</div>

		<!-- Networking Section -->
		<div class="space-y-4">
			<h3 class="text-sm font-semibold text-[var(--color-text)] uppercase tracking-wide">Networking</h3>
			
			<Input 
				type="number" 
				label="Port" 
				placeholder="8080"
				bind:value={serviceForm.port}
			/>
		</div>

		{#if serviceError}
			<div class="p-3 rounded-[var(--radius-md)] bg-[var(--color-error-light)] border border-[var(--color-error)]/30 text-[var(--color-error)] text-sm">
				{serviceError}
			</div>
		{/if}
	</form>

	{#snippet footer()}
		<Button variant="ghost" onclick={() => showServiceModal = false}>Cancel</Button>
		<Button type="submit" form="service-form" loading={creatingService}>
			{editingServiceName ? 'Save Changes' : 'Add Service'}
		</Button>
	{/snippet}
</Dialog>

<!-- Secret Modal -->
<Dialog
	bind:open={showSecretModal}
	title="Add Secret"
	description="Add an environment secret for your services"
>
	<form id="secret-form" onsubmit={createSecret} class="space-y-4">
		<Input label="Key" placeholder="DATABASE_URL" bind:value={secretKey} required />
		<Input type="password" label="Value" placeholder="••••••••" bind:value={secretValue} required />
	</form>

	{#snippet footer()}
		<Button variant="ghost" onclick={() => showSecretModal = false}>Cancel</Button>
		<Button type="submit" form="secret-form" loading={creatingSecret}>Add Secret</Button>
	{/snippet}
</Dialog>

<!-- Deploy Modal -->
<Dialog
	bind:open={showDeployModal}
	title={deployServiceName ? `Deploy ${deployServiceName}` : 'Deploy All Services'}
	description="Start a new deployment"
>
	<form id="deploy-form" onsubmit={deployApp} class="space-y-4">
		<Input 
			label="Git Ref (optional)" 
			placeholder="Leave empty to use service default" 
			bind:value={deployGitRef} 
		/>
	</form>

	{#snippet footer()}
		<Button variant="ghost" onclick={() => { showDeployModal = false; deployServiceName = ''; }}>
			Cancel
		</Button>
		<Button type="submit" form="deploy-form" loading={deploying}>
			<Play class="w-4 h-4" />
			Deploy
		</Button>
	{/snippet}
</Dialog>

<!-- Delete Confirmation Modal -->
<Dialog
	bind:open={showDeleteModal}
	title={deleteTarget ? `Delete ${deleteTarget.type}?` : 'Delete'}
	description={deleteTarget ? `Are you sure you want to delete "${deleteTarget.name}"? This action cannot be undone.` : ''}
>
	<p class="text-[var(--color-text-secondary)]">
		This will permanently delete the {deleteTarget?.type} and all associated data.
	</p>

	{#snippet footer()}
		<Button variant="ghost" onclick={() => { showDeleteModal = false; deleteTarget = null; }}>
			Cancel
		</Button>
		<Button variant="danger" loading={deleting} onclick={handleDelete}>
			<Trash2 class="w-4 h-4" />
			Delete
		</Button>
	{/snippet}
</Dialog>

<!-- Preview Modal -->
<Dialog
	bind:open={showPreviewModal}
	title={`Build Preview: ${previewServiceName}`}
	class="max-w-3xl"
>
	{#if previewLoading}
		<div class="flex items-center justify-center py-12">
			<div class="w-8 h-8 border-2 border-[var(--color-primary)] border-t-transparent rounded-full animate-spin"></div>
			<span class="ml-3 text-[var(--color-text-muted)]">Generating preview...</span>
		</div>
	{:else if previewError}
		<div class="p-4 rounded-[var(--radius-lg)] bg-[var(--color-error-light)] border border-[var(--color-error)]/30 text-[var(--color-error)]">
			{previewError}
		</div>
	{:else if previewResult}
		<div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
			<div class="p-3 rounded-[var(--radius-lg)] bg-[var(--color-background-subtle)]">
				<p class="text-xs text-[var(--color-text-muted)]">Strategy</p>
				<p class="font-medium text-[var(--color-text)]">{previewResult.strategy}</p>
			</div>
			<div class="p-3 rounded-[var(--radius-lg)] bg-[var(--color-background-subtle)]">
				<p class="text-xs text-[var(--color-text-muted)]">Build Type</p>
				<p class="font-medium text-[var(--color-text)]">{previewResult.build_type}</p>
			</div>
			<div class="p-3 rounded-[var(--radius-lg)] bg-[var(--color-background-subtle)]">
				<p class="text-xs text-[var(--color-text-muted)]">Est. Time</p>
				<p class="font-medium text-[var(--color-text)]">{Math.round(previewResult.estimated_build_time / 60)}m</p>
			</div>
			<div class="p-3 rounded-[var(--radius-lg)] bg-[var(--color-background-subtle)]">
				<p class="text-xs text-[var(--color-text-muted)]">Memory</p>
				<p class="font-medium text-[var(--color-text)]">{previewResult.estimated_resources.memory_mb}MB</p>
			</div>
		</div>

		{#if previewResult.generated_flake}
			<div>
				<div class="flex items-center justify-between mb-2">
					<h3 class="font-medium text-[var(--color-text)]">Generated flake.nix</h3>
					{#if previewResult.flake_valid !== undefined}
						<span class="text-sm {previewResult.flake_valid ? 'text-[var(--color-success)]' : 'text-[var(--color-error)]'}">
							{previewResult.flake_valid ? '✓ Valid' : '✗ ' + previewResult.validation_error}
						</span>
					{/if}
				</div>
				<pre class="p-4 rounded-[var(--radius-lg)] bg-[#0d1117] border border-[var(--color-border)] overflow-x-auto text-sm font-mono max-h-96 overflow-y-auto text-gray-200"><code>{previewResult.generated_flake}</code></pre>
			</div>
		{/if}
	{/if}

	{#snippet footer()}
		<Button variant="ghost" onclick={() => showPreviewModal = false}>Close</Button>
		<Button onclick={() => { showPreviewModal = false; deployServiceName = previewServiceName; showDeployModal = true; }}>
			<Play class="w-4 h-4" />
			Deploy
		</Button>
	{/snippet}
</Dialog>
