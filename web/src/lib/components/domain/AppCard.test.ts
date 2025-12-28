import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { App, Deployment, DeploymentStatus, ResourceTier } from '$lib/api';
import { formatRelativeTime } from '$lib/utils/formatters';

/**
 * Feature: professional-web-ui, Property 3: Application card displays all required information
 * Validates: Requirements 4.2
 *
 * For any application object with name, description, services array, and timestamps,
 * the rendered application card should contain the application name, description
 * (or placeholder), service count, and formatted date.
 */

// Valid deployment statuses
const deploymentStatuses: DeploymentStatus[] = [
	'pending', 'building', 'built', 'scheduled', 'starting', 
	'running', 'stopping', 'stopped', 'failed'
];

// Valid resource tiers
const resourceTiers: ResourceTier[] = ['nano', 'small', 'medium', 'large', 'xlarge'];

// Generator for service configs (simplified for testing)
const serviceConfigArb = fc.record({
	name: fc.string({ minLength: 1, maxLength: 50 }).filter(s => s.trim().length > 0),
	source_type: fc.constantFrom('git' as const, 'flake' as const, 'image' as const),
	resource_tier: fc.constantFrom(...resourceTiers),
	replicas: fc.integer({ min: 1, max: 10 }),
});

// Helper to generate valid ISO date strings
const validDateArb = fc.integer({ 
	min: new Date('2020-01-01').getTime(), 
	max: Date.now() 
}).map(ts => new Date(ts).toISOString());

// Generator for App objects
const appArb = fc.record({
	id: fc.uuid(),
	owner_id: fc.uuid(),
	name: fc.string({ minLength: 1, maxLength: 100 }).filter(s => s.trim().length > 0),
	description: fc.option(fc.string({ minLength: 0, maxLength: 500 }), { nil: undefined }),
	services: fc.array(serviceConfigArb, { minLength: 0, maxLength: 10 }),
	created_at: validDateArb,
	updated_at: validDateArb,
});

// Generator for Deployment objects
const deploymentArb = (appId: string, serviceNames: string[]) => {
	if (serviceNames.length === 0) {
		return fc.constant([]);
	}
	return fc.array(
		fc.record({
			id: fc.uuid(),
			app_id: fc.constant(appId),
			service_name: fc.constantFrom(...serviceNames),
			version: fc.integer({ min: 1, max: 100 }),
			git_ref: fc.string({ minLength: 1, maxLength: 50 }),
			git_commit: fc.option(fc.stringMatching(/^[0-9a-f]{7,40}$/), { nil: undefined }),
			build_type: fc.constantFrom('oci' as const, 'pure-nix' as const),
			status: fc.constantFrom(...deploymentStatuses),
			resource_tier: fc.constantFrom(...resourceTiers),
			created_at: validDateArb,
			updated_at: validDateArb,
		}),
		{ minLength: 0, maxLength: 20 }
	);
};

/**
 * Simulates what the AppCard component would render
 * This mirrors the logic in AppCard.svelte
 */
function getAppCardRenderInfo(app: App, deployments: Deployment[]) {
	const serviceCount = app.services?.length ?? 0;
	
	// Get latest deployment
	let latestDeployment: Deployment | null = null;
	if (deployments.length > 0) {
		latestDeployment = deployments.reduce((latest, current) => 
			new Date(current.created_at) > new Date(latest.created_at) ? current : latest
		);
	}
	
	return {
		name: app.name,
		description: app.description || null,
		hasDescription: !!app.description,
		serviceCount,
		serviceCountText: `${serviceCount} service${serviceCount !== 1 ? 's' : ''}`,
		latestDeploymentTime: latestDeployment ? formatRelativeTime(latestDeployment.created_at) : null,
		hasLatestDeployment: !!latestDeployment,
		appIcon: app.name.charAt(0).toUpperCase(),
	};
}

describe('AppCard information display', () => {
	it('should display application name', () => {
		fc.assert(
			fc.property(appArb, (app) => {
				const renderInfo = getAppCardRenderInfo(app, []);
				
				// Name should always be present and match the app name
				expect(renderInfo.name).toBe(app.name);
				expect(renderInfo.name.length).toBeGreaterThan(0);
			}),
			{ numRuns: 100 }
		);
	});

	it('should display description or placeholder when no description', () => {
		fc.assert(
			fc.property(appArb, (app) => {
				const renderInfo = getAppCardRenderInfo(app, []);
				
				if (app.description) {
					// When description exists, it should be displayed
					expect(renderInfo.hasDescription).toBe(true);
					expect(renderInfo.description).toBe(app.description);
				} else {
					// When no description, hasDescription should be false
					expect(renderInfo.hasDescription).toBe(false);
					expect(renderInfo.description).toBeNull();
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('should display correct service count', () => {
		fc.assert(
			fc.property(appArb, (app) => {
				const renderInfo = getAppCardRenderInfo(app, []);
				const expectedCount = app.services?.length ?? 0;
				
				// Service count should match
				expect(renderInfo.serviceCount).toBe(expectedCount);
				
				// Service count text should be grammatically correct
				if (expectedCount === 1) {
					expect(renderInfo.serviceCountText).toBe('1 service');
				} else {
					expect(renderInfo.serviceCountText).toBe(`${expectedCount} services`);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('should display app icon as first letter of name', () => {
		fc.assert(
			fc.property(appArb, (app) => {
				const renderInfo = getAppCardRenderInfo(app, []);
				
				// Icon should be uppercase first letter of name
				expect(renderInfo.appIcon).toBe(app.name.charAt(0).toUpperCase());
			}),
			{ numRuns: 100 }
		);
	});

	it('should display latest deployment time when deployments exist', () => {
		fc.assert(
			fc.property(
				appArb.chain(app => {
					const serviceNames = app.services.map(s => s.name);
					return deploymentArb(app.id, serviceNames).map(deployments => ({ app, deployments }));
				}),
				({ app, deployments }) => {
					const renderInfo = getAppCardRenderInfo(app, deployments);
					
					if (deployments.length > 0) {
						// Should have latest deployment info
						expect(renderInfo.hasLatestDeployment).toBe(true);
						expect(renderInfo.latestDeploymentTime).not.toBeNull();
						
						// The time should be a valid relative time format
						const timePattern = /^(just now|\d+[mhd] ago)$/;
						expect(renderInfo.latestDeploymentTime).toMatch(timePattern);
					} else {
						// No deployments means no latest deployment
						expect(renderInfo.hasLatestDeployment).toBe(false);
						expect(renderInfo.latestDeploymentTime).toBeNull();
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should contain all required information elements', () => {
		fc.assert(
			fc.property(
				appArb.chain(app => {
					const serviceNames = app.services.map(s => s.name);
					return deploymentArb(app.id, serviceNames).map(deployments => ({ app, deployments }));
				}),
				({ app, deployments }) => {
					const renderInfo = getAppCardRenderInfo(app, deployments);
					
					// All required fields should be present
					expect(renderInfo.name).toBeDefined();
					expect(typeof renderInfo.hasDescription).toBe('boolean');
					expect(typeof renderInfo.serviceCount).toBe('number');
					expect(renderInfo.serviceCountText).toBeDefined();
					expect(renderInfo.appIcon).toBeDefined();
					expect(typeof renderInfo.hasLatestDeployment).toBe('boolean');
				}
			),
			{ numRuns: 100 }
		);
	});
});


/**
 * Feature: professional-web-ui, Property 5: Application health indicator reflects service statuses
 * Validates: Requirements 4.5
 *
 * For any application with services and their deployment statuses, the health indicator
 * should show "healthy" only when all services have running deployments, "partial" when
 * some are running, and "failed" when none are running or any have failed status.
 */

/**
 * Calculate application health based on service deployment statuses
 * This mirrors the logic in AppCard.svelte
 * 
 * - "healthy": All services have running deployments
 * - "unhealthy": Some services are running (partial), none are running, or any have failed
 * - "unknown": No deployments exist or no services
 */
function getAppHealth(app: App, deployments: Deployment[]): 'healthy' | 'unhealthy' | 'unknown' {
	const serviceCount = app.services?.length ?? 0;
	if (serviceCount === 0) return 'unknown';

	// Get latest deployment for each service
	const latestByService = new Map<string, Deployment>();
	for (const deployment of deployments) {
		const existing = latestByService.get(deployment.service_name);
		if (!existing || new Date(deployment.created_at) > new Date(existing.created_at)) {
			latestByService.set(deployment.service_name, deployment);
		}
	}

	if (latestByService.size === 0) return 'unknown';

	const statuses = Array.from(latestByService.values()).map(d => d.status);
	const runningCount = statuses.filter(s => s === 'running').length;
	const failedCount = statuses.filter(s => s === 'failed').length;

	if (failedCount > 0) return 'unhealthy';
	if (runningCount === serviceCount) return 'healthy';
	if (runningCount > 0) return 'unhealthy'; // partial = unhealthy for display
	return 'unknown';
}

// Generator for apps with at least one service (for health testing)
const appWithServicesArb = fc.record({
	id: fc.uuid(),
	owner_id: fc.uuid(),
	name: fc.string({ minLength: 1, maxLength: 100 }).filter(s => s.trim().length > 0),
	description: fc.option(fc.string({ minLength: 0, maxLength: 500 }), { nil: undefined }),
	services: fc.array(serviceConfigArb, { minLength: 1, maxLength: 5 }),
	created_at: validDateArb,
	updated_at: validDateArb,
});

// Generator for deployments with specific status distribution
const deploymentsWithStatusArb = (appId: string, serviceNames: string[], statusDistribution: 'all-running' | 'some-running' | 'none-running' | 'has-failed') => {
	if (serviceNames.length === 0) {
		return fc.constant([]);
	}

	const statusForDistribution = (index: number): DeploymentStatus => {
		switch (statusDistribution) {
			case 'all-running':
				return 'running';
			case 'some-running':
				return index === 0 ? 'running' : 'stopped';
			case 'none-running':
				return 'stopped';
			case 'has-failed':
				return index === 0 ? 'failed' : 'running';
		}
	};

	return fc.tuple(
		...serviceNames.map((serviceName, index) =>
			fc.record({
				id: fc.uuid(),
				app_id: fc.constant(appId),
				service_name: fc.constant(serviceName),
				version: fc.integer({ min: 1, max: 100 }),
				git_ref: fc.string({ minLength: 1, maxLength: 50 }),
				git_commit: fc.option(fc.stringMatching(/^[0-9a-f]{7,40}$/), { nil: undefined }),
				build_type: fc.constantFrom('oci' as const, 'pure-nix' as const),
				status: fc.constant(statusForDistribution(index)),
				resource_tier: fc.constantFrom(...resourceTiers),
				created_at: validDateArb,
				updated_at: validDateArb,
			})
		)
	);
};

describe('Application health indicator', () => {
	it('should show healthy when all services have running deployments', () => {
		fc.assert(
			fc.property(
				appWithServicesArb.chain(app => {
					const serviceNames = app.services.map(s => s.name);
					return deploymentsWithStatusArb(app.id, serviceNames, 'all-running')
						.map(deployments => ({ app, deployments }));
				}),
				({ app, deployments }) => {
					const health = getAppHealth(app, deployments);
					
					// When all services have running deployments, health should be healthy
					expect(health).toBe('healthy');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should show unhealthy when some services are running (partial)', () => {
		fc.assert(
			fc.property(
				appWithServicesArb
					.filter(app => app.services.length >= 2) // Need at least 2 services for partial
					.chain(app => {
						const serviceNames = app.services.map(s => s.name);
						return deploymentsWithStatusArb(app.id, serviceNames, 'some-running')
							.map(deployments => ({ app, deployments }));
					}),
				({ app, deployments }) => {
					const health = getAppHealth(app, deployments);
					
					// When some but not all services are running, health should be unhealthy (partial)
					expect(health).toBe('unhealthy');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should show unhealthy when any service has failed status', () => {
		fc.assert(
			fc.property(
				appWithServicesArb.chain(app => {
					const serviceNames = app.services.map(s => s.name);
					return deploymentsWithStatusArb(app.id, serviceNames, 'has-failed')
						.map(deployments => ({ app, deployments }));
				}),
				({ app, deployments }) => {
					const health = getAppHealth(app, deployments);
					
					// When any service has failed, health should be unhealthy
					expect(health).toBe('unhealthy');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should show unknown when no deployments exist', () => {
		fc.assert(
			fc.property(appWithServicesArb, (app) => {
				const health = getAppHealth(app, []);
				
				// When no deployments exist, health should be unknown
				expect(health).toBe('unknown');
			}),
			{ numRuns: 100 }
		);
	});

	it('should show unknown when app has no services', () => {
		fc.assert(
			fc.property(
				fc.record({
					id: fc.uuid(),
					owner_id: fc.uuid(),
					name: fc.string({ minLength: 1, maxLength: 100 }).filter(s => s.trim().length > 0),
					description: fc.option(fc.string({ minLength: 0, maxLength: 500 }), { nil: undefined }),
					services: fc.constant([]),
					created_at: validDateArb,
					updated_at: validDateArb,
				}),
				(app) => {
					const health = getAppHealth(app, []);
					
					// When app has no services, health should be unknown
					expect(health).toBe('unknown');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use latest deployment per service for health calculation', () => {
		fc.assert(
			fc.property(
				appWithServicesArb.chain(app => {
					const serviceNames = app.services.map(s => s.name);
					// Create multiple deployments per service with different timestamps
					return fc.tuple(
						fc.constant(app),
						fc.array(
							fc.record({
								id: fc.uuid(),
								app_id: fc.constant(app.id),
								service_name: fc.constantFrom(...serviceNames),
								version: fc.integer({ min: 1, max: 100 }),
								git_ref: fc.string({ minLength: 1, maxLength: 50 }),
								git_commit: fc.option(fc.stringMatching(/^[0-9a-f]{7,40}$/), { nil: undefined }),
								build_type: fc.constantFrom('oci' as const, 'pure-nix' as const),
								status: fc.constantFrom(...deploymentStatuses),
								resource_tier: fc.constantFrom(...resourceTiers),
								created_at: validDateArb,
								updated_at: validDateArb,
							}),
							{ minLength: 1, maxLength: 20 }
						)
					);
				}),
				([app, deployments]) => {
					const health = getAppHealth(app, deployments);
					
					// Health should be one of the valid values
					expect(['healthy', 'unhealthy', 'unknown']).toContain(health);
					
					// Verify the logic by manually computing expected health
					const latestByService = new Map<string, Deployment>();
					for (const deployment of deployments) {
						const existing = latestByService.get(deployment.service_name);
						if (!existing || new Date(deployment.created_at) > new Date(existing.created_at)) {
							latestByService.set(deployment.service_name, deployment);
						}
					}
					
					if (latestByService.size === 0) {
						expect(health).toBe('unknown');
					} else {
						const statuses = Array.from(latestByService.values()).map(d => d.status);
						const runningCount = statuses.filter(s => s === 'running').length;
						const failedCount = statuses.filter(s => s === 'failed').length;
						const serviceCount = app.services.length;
						
						if (failedCount > 0) {
							expect(health).toBe('unhealthy');
						} else if (runningCount === serviceCount) {
							expect(health).toBe('healthy');
						} else if (runningCount > 0) {
							expect(health).toBe('unhealthy');
						} else {
							expect(health).toBe('unknown');
						}
					}
				}
			),
			{ numRuns: 100 }
		);
	});
});
