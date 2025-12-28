import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { calculateDashboardStats } from './dashboard-stats';
import type { App, Node, Deployment, DeploymentStatus, ServiceConfig, ResourceTier } from '$lib/api';

/**
 * Arbitrary generators for API types
 */

// Generate a valid resource tier
const resourceTierArb = fc.constantFrom<ResourceTier>('nano', 'small', 'medium', 'large', 'xlarge');

// Generate a valid deployment status
const deploymentStatusArb = fc.constantFrom<DeploymentStatus>(
	'pending', 'building', 'built', 'scheduled', 'starting', 'running', 'stopping', 'stopped', 'failed'
);

// Generate a service config
const serviceConfigArb: fc.Arbitrary<ServiceConfig> = fc.record({
	name: fc.string({ minLength: 1, maxLength: 20 }).filter(s => /^[a-z][a-z0-9-]*$/.test(s) || s.length === 0).map(s => s || 'service'),
	source_type: fc.constantFrom('git' as const, 'flake' as const, 'image' as const),
	resource_tier: resourceTierArb,
	replicas: fc.integer({ min: 1, max: 10 }),
});

// Generate an app with services
const appArb: fc.Arbitrary<App> = fc.record({
	id: fc.uuid(),
	owner_id: fc.uuid(),
	name: fc.string({ minLength: 1, maxLength: 50 }),
	description: fc.option(fc.string({ maxLength: 200 }), { nil: undefined }),
	services: fc.array(serviceConfigArb, { minLength: 0, maxLength: 10 }),
	created_at: fc.date().map(d => d.toISOString()),
	updated_at: fc.date().map(d => d.toISOString()),
});

// Generate a node
const nodeArb: fc.Arbitrary<Node> = fc.record({
	id: fc.uuid(),
	hostname: fc.string({ minLength: 1, maxLength: 50 }),
	address: fc.ipV4(),
	grpc_port: fc.integer({ min: 1024, max: 65535 }),
	healthy: fc.boolean(),
	resources: fc.option(fc.record({
		cpu_total: fc.integer({ min: 1000, max: 64000 }),
		cpu_available: fc.integer({ min: 0, max: 64000 }),
		memory_total: fc.integer({ min: 1024 * 1024 * 1024, max: 256 * 1024 * 1024 * 1024 }),
		memory_available: fc.integer({ min: 0, max: 256 * 1024 * 1024 * 1024 }),
		disk_total: fc.integer({ min: 10 * 1024 * 1024 * 1024, max: 1000 * 1024 * 1024 * 1024 }),
		disk_available: fc.integer({ min: 0, max: 1000 * 1024 * 1024 * 1024 }),
	}), { nil: undefined }),
	cached_paths: fc.option(fc.array(fc.string()), { nil: undefined }),
	last_heartbeat: fc.date().map(d => d.toISOString()),
	registered_at: fc.date().map(d => d.toISOString()),
});

// Generate a deployment
const deploymentArb: fc.Arbitrary<Deployment> = fc.record({
	id: fc.uuid(),
	app_id: fc.uuid(),
	service_name: fc.string({ minLength: 1, maxLength: 20 }),
	version: fc.integer({ min: 1, max: 1000 }),
	git_ref: fc.string({ minLength: 1, maxLength: 50 }),
	git_commit: fc.option(fc.stringMatching(/^[0-9a-f]{40}$/), { nil: undefined }),
	build_type: fc.constantFrom('oci' as const, 'pure-nix' as const),
	artifact: fc.option(fc.string(), { nil: undefined }),
	status: deploymentStatusArb,
	node_id: fc.option(fc.uuid(), { nil: undefined }),
	resource_tier: resourceTierArb,
	depends_on: fc.option(fc.array(fc.string()), { nil: undefined }),
	created_at: fc.date().map(d => d.toISOString()),
	updated_at: fc.date().map(d => d.toISOString()),
	started_at: fc.option(fc.date().map(d => d.toISOString()), { nil: undefined }),
	finished_at: fc.option(fc.date().map(d => d.toISOString()), { nil: undefined }),
});

describe('calculateDashboardStats', () => {
	/**
	 * Feature: professional-web-ui, Property 2: Dashboard statistics reflect actual data counts
	 * Validates: Requirements 3.1
	 * 
	 * For any set of applications, services, deployments, and nodes returned from the API,
	 * the dashboard statistics cards should display counts that exactly match the lengths
	 * of these collections.
	 */
	it('should calculate statistics that exactly match data counts', () => {
		fc.assert(
			fc.property(
				fc.array(appArb, { minLength: 0, maxLength: 20 }),
				fc.array(nodeArb, { minLength: 0, maxLength: 10 }),
				fc.array(deploymentArb, { minLength: 0, maxLength: 50 }),
				(apps, nodes, deployments) => {
					const stats = calculateDashboardStats(apps, nodes, deployments);

					// Total applications should equal apps array length
					expect(stats.totalApplications).toBe(apps.length);

					// Total services should equal sum of all services across apps
					const expectedServices = apps.reduce((sum, app) => sum + (app.services?.length ?? 0), 0);
					expect(stats.totalServices).toBe(expectedServices);

					// Running deployments should equal count of deployments with status 'running'
					const expectedRunning = deployments.filter(d => d.status === 'running').length;
					expect(stats.runningDeployments).toBe(expectedRunning);

					// Total nodes should equal nodes array length
					expect(stats.totalNodes).toBe(nodes.length);

					// Healthy nodes should equal count of nodes with healthy === true
					const expectedHealthy = nodes.filter(n => n.healthy).length;
					expect(stats.healthyNodes).toBe(expectedHealthy);
				}
			),
			{ numRuns: 100 }
		);
	});

	/**
	 * Edge case: Empty data should return all zeros
	 */
	it('should return zeros for empty data', () => {
		const stats = calculateDashboardStats([], [], []);
		
		expect(stats.totalApplications).toBe(0);
		expect(stats.totalServices).toBe(0);
		expect(stats.runningDeployments).toBe(0);
		expect(stats.totalNodes).toBe(0);
		expect(stats.healthyNodes).toBe(0);
	});

	/**
	 * Property: Healthy nodes should never exceed total nodes
	 */
	it('should have healthy nodes <= total nodes', () => {
		fc.assert(
			fc.property(
				fc.array(nodeArb, { minLength: 0, maxLength: 20 }),
				(nodes) => {
					const stats = calculateDashboardStats([], nodes, []);
					expect(stats.healthyNodes).toBeLessThanOrEqual(stats.totalNodes);
				}
			),
			{ numRuns: 100 }
		);
	});
});
