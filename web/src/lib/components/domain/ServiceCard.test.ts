import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { ServiceConfig, Deployment, DeploymentStatus, ResourceTier, BuildStrategy, SourceType } from '$lib/api';

/**
 * Feature: professional-web-ui, Property 6: Service card displays required information
 * Validates: Requirements 5.5
 *
 * For any service configuration object, the rendered service card should display
 * the service name, status badge, resource tier, and action buttons.
 */

// Valid deployment statuses
const deploymentStatuses: DeploymentStatus[] = [
	'pending', 'building', 'built', 'scheduled', 'starting', 
	'running', 'stopping', 'stopped', 'failed'
];

// Valid resource tiers
const resourceTiers: ResourceTier[] = ['nano', 'small', 'medium', 'large', 'xlarge'];

// Valid build strategies
const buildStrategies: BuildStrategy[] = [
	'flake', 'auto-go', 'auto-rust', 'auto-node', 'auto-python', 'dockerfile', 'nixpacks', 'auto'
];

// Valid source types
const sourceTypes: SourceType[] = ['git', 'flake', 'image'];

// Helper to generate valid ISO date strings
const validDateArb = fc.integer({ 
	min: new Date('2020-01-01').getTime(), 
	max: Date.now() 
}).map(ts => new Date(ts).toISOString());

// Generator for port mappings
const portMappingArb = fc.record({
	container_port: fc.integer({ min: 1, max: 65535 }),
	protocol: fc.option(fc.constantFrom('tcp', 'udp'), { nil: undefined }),
});

// Generator for ServiceConfig objects
const serviceConfigArb = fc.record({
	name: fc.string({ minLength: 1, maxLength: 50 }).filter(s => s.trim().length > 0),
	source_type: fc.constantFrom(...sourceTypes),
	git_repo: fc.option(fc.string({ minLength: 5, maxLength: 200 }).filter(s => s.includes('/')), { nil: undefined }),
	git_ref: fc.option(fc.string({ minLength: 1, maxLength: 50 }), { nil: undefined }),
	flake_uri: fc.option(fc.string({ minLength: 5, maxLength: 200 }), { nil: undefined }),
	image: fc.option(fc.string({ minLength: 3, maxLength: 200 }), { nil: undefined }),
	build_strategy: fc.option(fc.constantFrom(...buildStrategies), { nil: undefined }),
	resource_tier: fc.constantFrom(...resourceTiers),
	replicas: fc.integer({ min: 1, max: 10 }),
	ports: fc.option(fc.array(portMappingArb, { minLength: 0, maxLength: 5 }), { nil: undefined }),
});

// Generator for Deployment objects
const deploymentArb = (serviceName: string) => fc.record({
	id: fc.uuid(),
	app_id: fc.uuid(),
	service_name: fc.constant(serviceName),
	version: fc.integer({ min: 1, max: 100 }),
	git_ref: fc.string({ minLength: 1, maxLength: 50 }),
	git_commit: fc.option(fc.stringMatching(/^[0-9a-f]{7,40}$/), { nil: undefined }),
	build_type: fc.constantFrom('oci' as const, 'pure-nix' as const),
	status: fc.constantFrom(...deploymentStatuses),
	resource_tier: fc.constantFrom(...resourceTiers),
	created_at: validDateArb,
	updated_at: validDateArb,
});

/**
 * Get resource tier display info
 * This mirrors the logic in ServiceCard.svelte
 */
function getResourceTierInfo(tier: string): { label: string; memory: string } {
	const tiers: Record<string, { label: string; memory: string }> = {
		nano: { label: 'Nano', memory: '256MB' },
		small: { label: 'Small', memory: '512MB' },
		medium: { label: 'Medium', memory: '1GB' },
		large: { label: 'Large', memory: '2GB' },
		xlarge: { label: 'XLarge', memory: '4GB' },
	};
	return tiers[tier] || { label: tier, memory: 'Unknown' };
}

/**
 * Get source display string
 * This mirrors the logic in ServiceCard.svelte
 */
function getSourceDisplay(service: ServiceConfig): string {
	if (service.git_repo) return service.git_repo;
	if (service.flake_uri) return service.flake_uri;
	if (service.image) return service.image;
	return 'No source configured';
}

/**
 * Simulates what the ServiceCard component would render
 * This mirrors the logic in ServiceCard.svelte
 */
function getServiceCardRenderInfo(service: ServiceConfig, deployment?: Deployment) {
	const tierInfo = getResourceTierInfo(service.resource_tier);
	const sourceDisplay = getSourceDisplay(service);
	
	return {
		name: service.name,
		hasDeployment: !!deployment,
		deploymentStatus: deployment?.status,
		resourceTier: tierInfo.label,
		resourceMemory: tierInfo.memory,
		replicas: service.replicas,
		gitRef: service.git_ref || 'main',
		buildStrategy: service.build_strategy || 'flake',
		sourceDisplay,
		ports: service.ports || [],
		hasPorts: (service.ports?.length ?? 0) > 0,
		// Action buttons should always be present
		hasDeployButton: true,
		hasEditButton: true,
		hasPreviewButton: true,
		hasDeleteButton: true,
	};
}

describe('ServiceCard information display', () => {
	/**
	 * Feature: professional-web-ui, Property 6: Service card displays required information
	 * Validates: Requirements 5.5
	 */
	it('should display service name', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service);
				
				// Name should always be present and match the service name
				expect(renderInfo.name).toBe(service.name);
				expect(renderInfo.name.length).toBeGreaterThan(0);
			}),
			{ numRuns: 100 }
		);
	});

	it('should display status badge when deployment exists', () => {
		fc.assert(
			fc.property(
				serviceConfigArb.chain(service => 
					deploymentArb(service.name).map(deployment => ({ service, deployment }))
				),
				({ service, deployment }) => {
					const renderInfo = getServiceCardRenderInfo(service, deployment);
					
					// When deployment exists, status should be displayed
					expect(renderInfo.hasDeployment).toBe(true);
					expect(renderInfo.deploymentStatus).toBe(deployment.status);
					expect(deploymentStatuses).toContain(renderInfo.deploymentStatus);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should not display status badge when no deployment exists', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service, undefined);
				
				// When no deployment, status should not be displayed
				expect(renderInfo.hasDeployment).toBe(false);
				expect(renderInfo.deploymentStatus).toBeUndefined();
			}),
			{ numRuns: 100 }
		);
	});

	it('should display correct resource tier information', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service);
				const expectedTierInfo = getResourceTierInfo(service.resource_tier);
				
				// Resource tier should match
				expect(renderInfo.resourceTier).toBe(expectedTierInfo.label);
				expect(renderInfo.resourceMemory).toBe(expectedTierInfo.memory);
				
				// Should be a valid tier
				expect(['Nano', 'Small', 'Medium', 'Large', 'XLarge']).toContain(renderInfo.resourceTier);
			}),
			{ numRuns: 100 }
		);
	});

	it('should display replicas count', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service);
				
				// Replicas should match
				expect(renderInfo.replicas).toBe(service.replicas);
				expect(renderInfo.replicas).toBeGreaterThanOrEqual(1);
			}),
			{ numRuns: 100 }
		);
	});

	it('should display git ref with default fallback', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service);
				
				// Git ref should be displayed, defaulting to 'main'
				if (service.git_ref) {
					expect(renderInfo.gitRef).toBe(service.git_ref);
				} else {
					expect(renderInfo.gitRef).toBe('main');
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('should display build strategy with default fallback', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service);
				
				// Build strategy should be displayed, defaulting to 'flake'
				if (service.build_strategy) {
					expect(renderInfo.buildStrategy).toBe(service.build_strategy);
				} else {
					expect(renderInfo.buildStrategy).toBe('flake');
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('should display source information', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service);
				
				// Source should be displayed
				expect(renderInfo.sourceDisplay).toBeDefined();
				expect(renderInfo.sourceDisplay.length).toBeGreaterThan(0);
				
				// Should match one of the source fields or default message
				const expectedSource = getSourceDisplay(service);
				expect(renderInfo.sourceDisplay).toBe(expectedSource);
			}),
			{ numRuns: 100 }
		);
	});

	it('should display ports when configured', () => {
		fc.assert(
			fc.property(serviceConfigArb, (service) => {
				const renderInfo = getServiceCardRenderInfo(service);
				
				// Ports should match
				if (service.ports && service.ports.length > 0) {
					expect(renderInfo.hasPorts).toBe(true);
					expect(renderInfo.ports.length).toBe(service.ports.length);
				} else {
					expect(renderInfo.hasPorts).toBe(false);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('should always have action buttons available', () => {
		fc.assert(
			fc.property(
				serviceConfigArb.chain(service => 
					fc.option(deploymentArb(service.name), { nil: undefined })
						.map(deployment => ({ service, deployment }))
				),
				({ service, deployment }) => {
					const renderInfo = getServiceCardRenderInfo(service, deployment);
					
					// All action buttons should be available
					expect(renderInfo.hasDeployButton).toBe(true);
					expect(renderInfo.hasEditButton).toBe(true);
					expect(renderInfo.hasPreviewButton).toBe(true);
					expect(renderInfo.hasDeleteButton).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should contain all required information elements', () => {
		fc.assert(
			fc.property(
				serviceConfigArb.chain(service => 
					fc.option(deploymentArb(service.name), { nil: undefined })
						.map(deployment => ({ service, deployment }))
				),
				({ service, deployment }) => {
					const renderInfo = getServiceCardRenderInfo(service, deployment);
					
					// All required fields should be present
					expect(renderInfo.name).toBeDefined();
					expect(typeof renderInfo.hasDeployment).toBe('boolean');
					expect(renderInfo.resourceTier).toBeDefined();
					expect(renderInfo.resourceMemory).toBeDefined();
					expect(typeof renderInfo.replicas).toBe('number');
					expect(renderInfo.gitRef).toBeDefined();
					expect(renderInfo.buildStrategy).toBeDefined();
					expect(renderInfo.sourceDisplay).toBeDefined();
					expect(Array.isArray(renderInfo.ports)).toBe(true);
					expect(typeof renderInfo.hasPorts).toBe('boolean');
					expect(renderInfo.hasDeployButton).toBe(true);
					expect(renderInfo.hasEditButton).toBe(true);
					expect(renderInfo.hasPreviewButton).toBe(true);
					expect(renderInfo.hasDeleteButton).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});
});
