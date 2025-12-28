/**
 * Dashboard Statistics Utility
 * Requirements: 3.1
 * 
 * Calculates dashboard statistics from API data.
 */

import type { App, Node, Deployment } from '$lib/api';

export interface DashboardStats {
	totalApplications: number;
	totalServices: number;
	runningDeployments: number;
	totalNodes: number;
	healthyNodes: number;
}

/**
 * Calculate dashboard statistics from API data
 * 
 * @param apps - List of applications
 * @param nodes - List of nodes
 * @param deployments - List of deployments
 * @returns Dashboard statistics object
 */
export function calculateDashboardStats(
	apps: App[],
	nodes: Node[],
	deployments: Deployment[]
): DashboardStats {
	// Total applications is the length of the apps array
	const totalApplications = apps.length;

	// Total services is the sum of services across all apps
	const totalServices = apps.reduce((sum, app) => sum + (app.services?.length ?? 0), 0);

	// Running deployments are those with status 'running'
	const runningDeployments = deployments.filter(d => d.status === 'running').length;

	// Total nodes is the length of the nodes array
	const totalNodes = nodes.length;

	// Healthy nodes are those with healthy === true
	const healthyNodes = nodes.filter(n => n.healthy).length;

	return {
		totalApplications,
		totalServices,
		runningDeployments,
		totalNodes,
		healthyNodes,
	};
}
