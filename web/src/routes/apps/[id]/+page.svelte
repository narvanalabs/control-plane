<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { 
		apps, services, deployments, secrets, logs, detect, preview,
		type App, type ServiceConfig, type Deployment, type LogEntry,
		type BuildStrategy, type DetectResponse, type PreviewResponse,
		APIError
	} from '$lib/api';
	import { Card, Button, Input, StatusBadge } from '$lib/components';

	// State
	let app = $state<App | null>(null);
	let deploymentList = $state<Deployment[]>([]);
	let secretKeys = $state<string[]>([]);
	let logEntries = $state<LogEntry[]>([]);
	let loading = $state(true);
	let activeTab = $state<'services' | 'deployments' | 'secrets' | 'logs'>('services');
	let error = $state('');

	// Service modal
	let showServiceModal = $state(false);
	let serviceForm = $state({
		name: '',
		git_repo: '',
		git_ref: 'main',
		build_strategy: 'auto' as BuildStrategy,
		resource_tier: 'small' as const,
		replicas: 1,
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

	const appId = $derived($page.params.id);

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
			// Auto-select detected strategy
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
		// First load existing logs
		try {
			const logsData = await logs.get(appId, {
				limit: 500,
				source: logSource === 'all' ? undefined : logSource,
				deployment_id: selectedDeploymentId || undefined,
			});
			// Show oldest first (chronological order)
			logEntries = (logsData.logs || []).reverse();
			
			// Scroll to bottom after loading
			requestAnimationFrame(() => {
				if (logContainer) {
					logContainer.scrollTop = logContainer.scrollHeight;
				}
			});
		} catch (err) {
			console.error('Failed to load logs:', err);
		}
		
		// Then start streaming for new logs
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
		
		// Track which log IDs we've seen to avoid duplicates
		const seenLogIds = new Set(logEntries.map(l => l.id));
		
		logEventSource.addEventListener('connected', (e) => {
			const data = JSON.parse((e as MessageEvent).data);
			console.log('Log stream connected:', data);
		});
		
		logEventSource.addEventListener('log', (e) => {
			const log = JSON.parse((e as MessageEvent).data);
			
			// Skip duplicates
			if (seenLogIds.has(log.id)) return;
			seenLogIds.add(log.id);
			
			logEntries = [...logEntries, log];
			
			// Auto-scroll to bottom
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
			// Don't stop - keep connection open for next deployment
		});
		
		logEventSource.onerror = (err) => {
			console.error('Log stream error:', err);
			// Will automatically reconnect
		};
	}

	// Stop log streaming
	function stopLogStream() {
		if (logEventSource) {
			logEventSource.close();
			logEventSource = null;
		}
		logStreaming = false;
	}

	// Cleanup on unmount
	$effect(() => {
		return () => {
			stopLogStream();
		};
	});

	// Auto-start streaming when logs tab is active
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

			// Load logs for most recent deployment
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

	async function createService(e: Event) {
		e.preventDefault();
		serviceError = '';
		creatingService = true;
		try {
			await services.create(appId, {
				name: serviceForm.name,
				git_repo: serviceForm.git_repo,
				git_ref: serviceForm.git_ref,
				build_strategy: serviceForm.build_strategy,
				resource_tier: serviceForm.resource_tier,
				replicas: serviceForm.replicas,
			});
			showServiceModal = false;
			serviceForm = { name: '', git_repo: '', git_ref: 'main', build_strategy: 'auto', resource_tier: 'small', replicas: 1 };
			detectResult = null;
			await loadAppData();
		} catch (err) {
			serviceError = err instanceof Error ? err.message : 'Failed to create service';
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

	function formatRelativeTime(date: string): string {
		const seconds = Math.floor((Date.now() - new Date(date).getTime()) / 1000);
		if (seconds < 60) return 'just now';
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
		return `${Math.floor(seconds / 86400)}d ago`;
	}

	function getServiceStatus(serviceName: string): Deployment | undefined {
		return deploymentList.find(d => d.service_name === serviceName);
	}

	const tabs = $derived([
		{ id: 'services' as const, label: 'Services', count: app?.services?.length ?? 0 },
		{ id: 'deployments' as const, label: 'Deployments', count: deploymentList.length },
		{ id: 'secrets' as const, label: 'Secrets', count: secretKeys.length },
		{ id: 'logs' as const, label: 'Logs', count: logEntries.length },
	]);
</script>

<div class="p-8">
	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="w-8 h-8 border-2 border-[var(--color-narvana-primary)] border-t-transparent rounded-full animate-spin"></div>
		</div>
	{:else if error}
		<Card class="p-12 text-center">
			<p class="text-red-400 mb-4">{error}</p>
			<Button onclick={() => goto('/apps')}>Back to Apps</Button>
		</Card>
	{:else if app}
		<!-- Header -->
		<div class="flex items-start justify-between mb-8">
			<div class="flex items-center gap-4">
				<a href="/apps" class="p-2 rounded-lg hover:bg-[var(--color-narvana-surface-hover)] text-[var(--color-narvana-text-dim)]">
					←
				</a>
				<div class="w-14 h-14 rounded-xl bg-gradient-to-br from-[var(--color-narvana-primary)]/20 to-[var(--color-narvana-secondary)]/20 flex items-center justify-center">
					<span class="text-2xl font-bold text-[var(--color-narvana-primary)]">
						{app.name.charAt(0).toUpperCase()}
					</span>
				</div>
				<div>
					<h1 class="text-2xl font-bold">{app.name}</h1>
					{#if app.description}
						<p class="text-[var(--color-narvana-text-dim)]">{app.description}</p>
					{/if}
				</div>
			</div>
			<div class="flex gap-3">
				<Button 
					variant="secondary" 
					onclick={() => { showDeployModal = true; }}
				>
					▶ Deploy
				</Button>
				<Button 
					variant="danger" 
					onclick={() => { deleteTarget = { type: 'app', name: app.name }; showDeleteModal = true; }}
				>
					Delete
				</Button>
			</div>
		</div>

		<!-- Tabs -->
		<div class="flex gap-1 mb-6 border-b border-[var(--color-narvana-border)]">
			{#each tabs as tab}
				<button
					class="px-4 py-3 text-sm font-medium border-b-2 transition-colors
						{activeTab === tab.id 
							? 'border-[var(--color-narvana-primary)] text-[var(--color-narvana-primary)]' 
							: 'border-transparent text-[var(--color-narvana-text-dim)] hover:text-[var(--color-narvana-text)]'}"
					onclick={() => activeTab = tab.id}
				>
					{tab.label}
					{#if tab.count !== undefined && tab.count > 0}
						<span class="ml-1.5 px-1.5 py-0.5 rounded-full bg-[var(--color-narvana-border)] text-xs">
							{tab.count}
						</span>
					{/if}
				</button>
			{/each}
		</div>

		<!-- Tab Content -->
		{#if activeTab === 'services'}
			<div class="flex justify-between items-center mb-4">
				<h2 class="text-lg font-semibold">Services</h2>
				<Button size="sm" onclick={() => showServiceModal = true}>
					+ Add Service
				</Button>
			</div>

			{#if (app.services?.length ?? 0) === 0}
				<Card class="p-8 text-center">
					<p class="text-[var(--color-narvana-text-dim)] mb-4">No services defined</p>
					<Button onclick={() => showServiceModal = true}>Add First Service</Button>
				</Card>
			{:else}
				<div class="space-y-4">
					{#each app.services ?? [] as service}
						{@const deployment = getServiceStatus(service.name)}
						<Card class="p-6">
							<div class="flex items-start justify-between">
								<div class="flex items-center gap-4">
									<div class="w-10 h-10 rounded-lg bg-[var(--color-narvana-secondary)]/10 flex items-center justify-center text-[var(--color-narvana-secondary)]">
										◇
									</div>
									<div>
										<h3 class="font-semibold">{service.name}</h3>
										<p class="text-sm text-[var(--color-narvana-text-muted)] font-mono">
											{service.git_repo || service.flake_uri || service.image || 'No source'}
										</p>
									</div>
								</div>
								<div class="flex items-center gap-2">
									{#if deployment}
										<StatusBadge status={deployment.status} size="sm" />
									{/if}
									<Button 
										size="sm" 
										variant="ghost"
										onclick={() => runPreview(service.name)}
									>
										👁 Preview
									</Button>
									<Button 
										size="sm" 
										variant="secondary"
										onclick={() => { deployServiceName = service.name; showDeployModal = true; }}
									>
										▶ Deploy
									</Button>
									<Button 
										size="sm" 
										variant="ghost"
										onclick={() => { deleteTarget = { type: 'service', name: service.name }; showDeleteModal = true; }}
									>
										×
									</Button>
								</div>
							</div>

							<div class="mt-4 grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Resource Tier</span>
									<p class="font-medium capitalize">{service.resource_tier}</p>
								</div>
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Replicas</span>
									<p class="font-medium">{service.replicas}</p>
								</div>
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Git Ref</span>
									<p class="font-medium font-mono">{service.git_ref || 'main'}</p>
								</div>
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Build Strategy</span>
									<p class="font-medium">{service.build_strategy || 'flake'}</p>
								</div>
							</div>

							{#if service.ports && service.ports.length > 0}
								<div class="mt-3 flex gap-2">
									{#each service.ports as port}
										<span class="px-2 py-1 rounded bg-[var(--color-narvana-bg)] text-xs font-mono">
											:{port.container_port}/{port.protocol || 'tcp'}
										</span>
									{/each}
								</div>
							{/if}
						</Card>
					{/each}
				</div>
			{/if}

		{:else if activeTab === 'deployments'}
			<h2 class="text-lg font-semibold mb-4">Deployment History</h2>
			{#if deploymentList.length === 0}
				<Card class="p-8 text-center">
					<p class="text-[var(--color-narvana-text-dim)]">No deployments yet</p>
				</Card>
			{:else}
				<div class="space-y-3">
					{#each deploymentList as deployment}
						<Card class="p-4">
							<div class="flex items-center justify-between">
								<div class="flex items-center gap-4">
									<StatusBadge status={deployment.status} />
									<div>
										<p class="font-medium">
											{deployment.service_name}
											<span class="text-[var(--color-narvana-text-muted)] font-normal">
												#{deployment.version}
											</span>
										</p>
										<p class="text-sm text-[var(--color-narvana-text-muted)]">
											{formatDate(deployment.created_at)}
											{#if deployment.git_ref}
												• <span class="font-mono">{deployment.git_ref}</span>
											{/if}
										</p>
									</div>
								</div>
								<div class="text-sm text-[var(--color-narvana-text-dim)]">
									{formatRelativeTime(deployment.created_at)}
								</div>
							</div>
						</Card>
					{/each}
				</div>
			{/if}

		{:else if activeTab === 'secrets'}
			<div class="flex justify-between items-center mb-4">
				<h2 class="text-lg font-semibold">Environment Secrets</h2>
				<Button size="sm" onclick={() => showSecretModal = true}>
					+ Add Secret
				</Button>
			</div>

			{#if secretKeys.length === 0}
				<Card class="p-8 text-center">
					<p class="text-[var(--color-narvana-text-dim)] mb-4">No secrets defined</p>
					<Button onclick={() => showSecretModal = true}>Add First Secret</Button>
				</Card>
			{:else}
				<div class="space-y-2">
					{#each secretKeys as key}
						<Card class="p-4 flex items-center justify-between">
							<div class="flex items-center gap-3">
								<span class="text-[var(--color-narvana-warning)]">🔐</span>
								<code class="font-mono text-sm">{key}</code>
							</div>
							<Button 
								size="sm" 
								variant="ghost"
								onclick={() => { deleteTarget = { type: 'secret', name: key }; showDeleteModal = true; }}
							>
								×
							</Button>
						</Card>
					{/each}
				</div>
			{/if}

		{:else if activeTab === 'logs'}
			<div class="flex items-center justify-between mb-4">
				<div class="flex items-center gap-3">
					<h2 class="text-lg font-semibold">Build & Runtime Logs</h2>
					{#if logStreaming}
						<span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-green-500/20 text-green-400 text-xs font-medium">
							<span class="w-2 h-2 rounded-full bg-green-400 animate-pulse"></span>
							Live
						</span>
					{/if}
				</div>
				<div class="flex gap-2 items-center">
					<!-- Source Filter -->
					<select
						bind:value={logSource}
						onchange={() => loadAndStreamLogs()}
						class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-narvana-bg)] border border-[var(--color-narvana-border)] text-[var(--color-narvana-text)]"
					>
						<option value="all">All Sources</option>
						<option value="build">Build Logs</option>
						<option value="runtime">Runtime Logs</option>
					</select>

					<!-- Deployment Filter -->
					{#if deploymentList.length > 0}
						<select
							bind:value={selectedDeploymentId}
							onchange={() => loadAndStreamLogs()}
							class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-narvana-bg)] border border-[var(--color-narvana-border)] text-[var(--color-narvana-text)]"
						>
							<option value="">Latest Deployment</option>
							{#each deploymentList.slice(0, 10) as dep}
								<option value={dep.id}>
									{dep.service_name} #{dep.version} ({dep.status})
								</option>
							{/each}
						</select>
					{/if}

					<!-- Auto-scroll toggle -->
					<label class="flex items-center gap-1.5 text-sm text-[var(--color-narvana-text-muted)] cursor-pointer">
						<input 
							type="checkbox" 
							bind:checked={autoScroll}
							class="w-4 h-4 rounded accent-[var(--color-narvana-primary)]"
						/>
						Auto-scroll
					</label>
				</div>
			</div>

			{#if logEntries.length === 0}
				<Card class="p-8 text-center bg-[#0d1117]">
					{#if logStreaming}
						<div class="flex items-center justify-center gap-2 text-[var(--color-narvana-text-muted)]">
							<span class="w-2 h-2 rounded-full bg-green-400 animate-pulse"></span>
							Waiting for logs...
						</div>
						<p class="text-sm text-[var(--color-narvana-text-muted)] mt-2">
							Deploy a service to see build output here in real-time.
						</p>
					{:else}
						<p class="text-[var(--color-narvana-text-dim)]">No logs available</p>
						<p class="text-sm text-[var(--color-narvana-text-muted)] mt-2">
							Deploy a service to see build and runtime logs here.
						</p>
					{/if}
				</Card>
			{:else}
				<Card class="p-0 overflow-hidden bg-[#0d1117]">
					<!-- Terminal-style log viewer -->
					<div 
						bind:this={logContainer}
						class="h-[600px] overflow-y-auto font-mono text-sm p-4 scroll-smooth"
					>
						{#if logEntries.length === 0 && logStreaming}
							<div class="flex items-center gap-2 text-[var(--color-narvana-text-muted)]">
								<span class="w-2 h-2 rounded-full bg-green-400 animate-pulse"></span>
								Waiting for logs...
							</div>
						{:else}
							{#each logEntries as log, i}
								<div 
									class="flex py-0.5 hover:bg-white/5 group
										{log.level === 'error' ? 'bg-red-500/10' : ''}"
								>
									<!-- Line number -->
									<span class="w-12 flex-shrink-0 text-right pr-4 text-[var(--color-narvana-text-muted)]/50 select-none">
										{i + 1}
									</span>
									<!-- Timestamp -->
									<span class="w-24 flex-shrink-0 text-[var(--color-narvana-text-muted)]">
										{new Date(log.timestamp).toLocaleTimeString('en-US', { hour12: false })}
									</span>
									<!-- Level with color -->
									<span class="w-12 flex-shrink-0 font-semibold
										{log.level === 'error' ? 'text-red-400' : 
										 log.level === 'warn' ? 'text-yellow-400' : 
										 log.level === 'info' ? 'text-cyan-400' : 
										 log.level === 'debug' ? 'text-purple-400' :
										 'text-[var(--color-narvana-text-dim)]'}">
										{log.level.toUpperCase().slice(0, 4)}
									</span>
									<!-- Source badge -->
									<span class="w-16 flex-shrink-0">
										<span class="inline-block px-1.5 py-0.5 rounded text-xs
											{log.source === 'build' ? 'bg-blue-500/20 text-blue-400' : 'bg-purple-500/20 text-purple-400'}">
											{log.source || 'sys'}
										</span>
									</span>
									<!-- Message -->
									<span class="flex-1 text-[var(--color-narvana-text)] whitespace-pre-wrap break-all">
										{log.message}
									</span>
								</div>
							{/each}
							{#if logStreaming}
								<div class="flex items-center gap-2 text-[var(--color-narvana-text-muted)] mt-2 pt-2 border-t border-white/10">
									<span class="w-2 h-2 rounded-full bg-green-400 animate-pulse"></span>
									Streaming...
								</div>
							{/if}
						{/if}
					</div>
				</Card>
				<div class="flex items-center justify-between mt-2">
					<p class="text-sm text-[var(--color-narvana-text-muted)]">
						{logEntries.length} log {logEntries.length === 1 ? 'entry' : 'entries'}
						{#if logStreaming}
							• Live streaming active
						{/if}
					</p>
					{#if logEntries.length > 0}
						<button 
							onclick={() => { if (logContainer) logContainer.scrollTop = logContainer.scrollHeight; }}
							class="text-sm text-[var(--color-narvana-primary)] hover:underline"
						>
							↓ Jump to bottom
						</button>
					{/if}
				</div>
			{/if}
		{/if}
	{/if}
</div>

<!-- Service Modal -->
{#if showServiceModal}
	<div 
		class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4 overflow-y-auto"
		onclick={(e) => { if (e.target === e.currentTarget) showServiceModal = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showServiceModal = false; }}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<Card class="w-full max-w-xl p-6 animate-slide-up my-8">
			<h2 class="text-xl font-semibold mb-4">Add Service</h2>
			
			<form onsubmit={createService} class="space-y-4">
				<Input label="Service Name" placeholder="api" bind:value={serviceForm.name} required />
				
				<!-- Git Repository with Detect button -->
				<div class="space-y-1.5">
					<label for="git-repo" class="block text-sm font-medium text-[var(--color-narvana-text-dim)]">
						Git Repository <span class="text-red-400">*</span>
					</label>
					<div class="flex gap-2">
						<input
							id="git-repo"
							type="text"
							placeholder="github.com/org/repo"
							bind:value={serviceForm.git_repo}
							required
							class="flex-1 px-4 py-2.5 rounded-lg bg-[var(--color-narvana-bg)] border border-[var(--color-narvana-border)] text-[var(--color-narvana-text)] focus:outline-none focus:border-[var(--color-narvana-primary)]"
						/>
						<Button 
							type="button" 
							variant="secondary" 
							onclick={runDetection}
							loading={detecting}
							disabled={!serviceForm.git_repo}
						>
							🔍 Detect
						</Button>
					</div>
				</div>

				<Input label="Git Ref" placeholder="main" bind:value={serviceForm.git_ref} />

				<!-- Detection Result -->
				{#if detectResult}
					<div class="p-4 rounded-lg bg-[var(--color-narvana-primary-glow)] border border-[var(--color-narvana-primary)]/30">
						<div class="flex items-center gap-2 mb-2">
							<span class="text-[var(--color-narvana-primary)]">✓</span>
							<span class="font-medium">Detection Result</span>
							<span class="text-sm text-[var(--color-narvana-text-muted)]">
								({Math.round(detectResult.confidence * 100)}% confidence)
							</span>
						</div>
						<div class="grid grid-cols-2 gap-2 text-sm">
							<div>
								<span class="text-[var(--color-narvana-text-muted)]">Framework:</span>
								<span class="ml-1 font-medium">{detectResult.framework || 'Unknown'}</span>
							</div>
							<div>
								<span class="text-[var(--color-narvana-text-muted)]">Strategy:</span>
								<span class="ml-1 font-medium">{detectResult.strategy}</span>
							</div>
							{#if detectResult.version}
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Version:</span>
									<span class="ml-1 font-mono">{detectResult.version}</span>
								</div>
							{/if}
							{#if detectResult.entry_points?.length}
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Entry:</span>
									<span class="ml-1 font-mono">{detectResult.entry_points[0]}</span>
								</div>
							{/if}
						</div>
						{#if detectResult.warnings?.length}
							<div class="mt-2 text-sm text-yellow-400">
								⚠ {detectResult.warnings.join(', ')}
							</div>
						{/if}
					</div>
				{/if}

				<!-- Build Strategy Selection -->
				<div>
					<label for="build-strategy" class="block text-sm font-medium text-[var(--color-narvana-text-dim)] mb-1.5">
						Build Strategy
					</label>
					<select 
						id="build-strategy"
						bind:value={serviceForm.build_strategy}
						class="w-full px-4 py-2.5 rounded-lg bg-[var(--color-narvana-bg)] border border-[var(--color-narvana-border)] text-[var(--color-narvana-text)]"
					>
						{#each buildStrategies as strategy}
							<option value={strategy.value}>{strategy.label} - {strategy.description}</option>
						{/each}
					</select>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="resource-tier" class="block text-sm font-medium text-[var(--color-narvana-text-dim)] mb-1.5">
							Resource Tier
						</label>
						<select 
							id="resource-tier"
							bind:value={serviceForm.resource_tier}
							class="w-full px-4 py-2.5 rounded-lg bg-[var(--color-narvana-bg)] border border-[var(--color-narvana-border)] text-[var(--color-narvana-text)]"
						>
							<option value="nano">Nano (256MB)</option>
							<option value="small">Small (512MB)</option>
							<option value="medium">Medium (1GB)</option>
							<option value="large">Large (2GB)</option>
							<option value="xlarge">XLarge (4GB)</option>
						</select>
					</div>
					<Input 
						type="number" 
						label="Replicas" 
						bind:value={serviceForm.replicas}
					/>
				</div>

				{#if serviceError}
					<div class="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-400 text-sm">
						{serviceError}
					</div>
				{/if}

				<div class="flex gap-3 pt-2">
					<Button variant="ghost" class="flex-1" onclick={() => showServiceModal = false}>Cancel</Button>
					<Button type="submit" class="flex-1" loading={creatingService}>Add Service</Button>
				</div>
			</form>
		</Card>
	</div>
{/if}

<!-- Secret Modal -->
{#if showSecretModal}
	<div 
		class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4"
		onclick={(e) => { if (e.target === e.currentTarget) showSecretModal = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showSecretModal = false; }}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<Card class="w-full max-w-md p-6 animate-slide-up">
			<h2 class="text-xl font-semibold mb-4">Add Secret</h2>
			
			<form onsubmit={createSecret} class="space-y-4">
				<Input label="Key" placeholder="DATABASE_URL" bind:value={secretKey} required />
				<Input type="password" label="Value" placeholder="••••••••" bind:value={secretValue} required />

				<div class="flex gap-3 pt-2">
					<Button variant="ghost" class="flex-1" onclick={() => showSecretModal = false}>Cancel</Button>
					<Button type="submit" class="flex-1" loading={creatingSecret}>Add Secret</Button>
				</div>
			</form>
		</Card>
	</div>
{/if}

<!-- Deploy Modal -->
{#if showDeployModal}
	<div 
		class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4"
		onclick={(e) => { if (e.target === e.currentTarget) showDeployModal = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showDeployModal = false; }}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<Card class="w-full max-w-md p-6 animate-slide-up">
			<h2 class="text-xl font-semibold mb-4">
				{deployServiceName ? `Deploy ${deployServiceName}` : 'Deploy All Services'}
			</h2>
			
			<form onsubmit={deployApp} class="space-y-4">
				<Input 
					label="Git Ref (optional)" 
					placeholder="Leave empty to use service default" 
					bind:value={deployGitRef} 
				/>

				<div class="flex gap-3 pt-2">
					<Button variant="ghost" class="flex-1" onclick={() => { showDeployModal = false; deployServiceName = ''; }}>
						Cancel
					</Button>
					<Button type="submit" class="flex-1" loading={deploying}>
						Deploy
					</Button>
				</div>
			</form>
		</Card>
	</div>
{/if}

<!-- Delete Confirmation Modal -->
{#if showDeleteModal && deleteTarget}
	<div 
		class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4"
		onclick={(e) => { if (e.target === e.currentTarget) showDeleteModal = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showDeleteModal = false; }}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<Card class="w-full max-w-md p-6 animate-slide-up">
			<h2 class="text-xl font-semibold mb-2">Delete {deleteTarget.type}?</h2>
			<p class="text-[var(--color-narvana-text-dim)] mb-6">
				Are you sure you want to delete <strong>{deleteTarget.name}</strong>? This action cannot be undone.
			</p>

			<div class="flex gap-3">
				<Button variant="ghost" class="flex-1" onclick={() => { showDeleteModal = false; deleteTarget = null; }}>
					Cancel
				</Button>
				<Button variant="danger" class="flex-1" loading={deleting} onclick={handleDelete}>
					Delete
				</Button>
			</div>
		</Card>
	</div>
{/if}

<!-- Preview Modal -->
{#if showPreviewModal}
	<div 
		class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4 overflow-y-auto"
		onclick={(e) => { if (e.target === e.currentTarget) showPreviewModal = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showPreviewModal = false; }}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<Card class="w-full max-w-3xl p-6 animate-slide-up my-8">
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-xl font-semibold">Build Preview: {previewServiceName}</h2>
				<button 
					onclick={() => showPreviewModal = false}
					class="text-[var(--color-narvana-text-muted)] hover:text-[var(--color-narvana-text)]"
				>
					✕
				</button>
			</div>

			{#if previewLoading}
				<div class="flex items-center justify-center py-12">
					<div class="w-8 h-8 border-2 border-[var(--color-narvana-primary)] border-t-transparent rounded-full animate-spin"></div>
					<span class="ml-3 text-[var(--color-narvana-text-dim)]">Generating preview...</span>
				</div>
			{:else if previewError}
				<div class="p-4 rounded-lg bg-red-500/10 border border-red-500/30 text-red-400">
					{previewError}
				</div>
			{:else if previewResult}
				<!-- Strategy & Build Info -->
				<div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
					<div class="p-3 rounded-lg bg-[var(--color-narvana-bg)]">
						<p class="text-xs text-[var(--color-narvana-text-muted)]">Strategy</p>
						<p class="font-medium">{previewResult.strategy}</p>
					</div>
					<div class="p-3 rounded-lg bg-[var(--color-narvana-bg)]">
						<p class="text-xs text-[var(--color-narvana-text-muted)]">Build Type</p>
						<p class="font-medium">{previewResult.build_type}</p>
					</div>
					<div class="p-3 rounded-lg bg-[var(--color-narvana-bg)]">
						<p class="text-xs text-[var(--color-narvana-text-muted)]">Est. Time</p>
						<p class="font-medium">{Math.round(previewResult.estimated_build_time / 60)}m</p>
					</div>
					<div class="p-3 rounded-lg bg-[var(--color-narvana-bg)]">
						<p class="text-xs text-[var(--color-narvana-text-muted)]">Memory</p>
						<p class="font-medium">{previewResult.estimated_resources.memory_mb}MB</p>
					</div>
				</div>

				<!-- Detection Info -->
				{#if previewResult.detection}
					<div class="mb-4 p-4 rounded-lg bg-[var(--color-narvana-primary-glow)] border border-[var(--color-narvana-primary)]/30">
						<p class="text-sm font-medium mb-2">🔍 Detected</p>
						<div class="grid grid-cols-3 gap-4 text-sm">
							<div>
								<span class="text-[var(--color-narvana-text-muted)]">Framework:</span>
								<span class="ml-1">{previewResult.detection.framework}</span>
							</div>
							{#if previewResult.detection.version}
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Version:</span>
									<span class="ml-1 font-mono">{previewResult.detection.version}</span>
								</div>
							{/if}
							{#if previewResult.detection.entry_point}
								<div>
									<span class="text-[var(--color-narvana-text-muted)]">Entry:</span>
									<span class="ml-1 font-mono">{previewResult.detection.entry_point}</span>
								</div>
							{/if}
						</div>
					</div>
				{/if}

				<!-- Warnings -->
				{#if previewResult.warnings?.length}
					<div class="mb-4 p-3 rounded-lg bg-yellow-500/10 border border-yellow-500/30">
						<p class="text-sm text-yellow-400">
							⚠ {previewResult.warnings.join(' • ')}
						</p>
					</div>
				{/if}

				<!-- Generated Flake -->
				{#if previewResult.generated_flake}
					<div>
						<div class="flex items-center justify-between mb-2">
							<h3 class="font-medium">Generated flake.nix</h3>
							{#if previewResult.flake_valid !== undefined}
								<span class="text-sm {previewResult.flake_valid ? 'text-green-400' : 'text-red-400'}">
									{previewResult.flake_valid ? '✓ Valid' : '✗ ' + previewResult.validation_error}
								</span>
							{/if}
						</div>
						<pre class="p-4 rounded-lg bg-[var(--color-narvana-bg)] border border-[var(--color-narvana-border)] overflow-x-auto text-sm font-mono max-h-96 overflow-y-auto"><code>{previewResult.generated_flake}</code></pre>
					</div>
				{:else}
					<div class="p-8 text-center text-[var(--color-narvana-text-dim)]">
						<p>No flake preview available for this build strategy.</p>
						<p class="text-sm mt-2">The build will use the existing Dockerfile or repository flake.nix.</p>
					</div>
				{/if}
			{/if}

			<div class="flex justify-end gap-3 mt-6 pt-4 border-t border-[var(--color-narvana-border)]">
				<Button variant="ghost" onclick={() => showPreviewModal = false}>Close</Button>
				<Button onclick={() => { showPreviewModal = false; deployServiceName = previewServiceName; showDeployModal = true; }}>
					▶ Deploy
				</Button>
			</div>
		</Card>
	</div>
{/if}

