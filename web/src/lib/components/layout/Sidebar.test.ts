import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { isActive } from './navigation';

/**
 * Feature: professional-web-ui, Property 1: Active navigation item matches current route
 * Validates: Requirements 2.5
 * 
 * For any route path in the application, the sidebar navigation item that matches
 * that path should have active styling applied, and all other navigation items
 * should not have active styling.
 */
describe('Sidebar Navigation', () => {
	const navItems = [
		{ href: '/dashboard', label: 'Dashboard' },
		{ href: '/apps', label: 'Applications' },
		{ href: '/nodes', label: 'Infrastructure' },
	];

	describe('isActive function', () => {
		/**
		 * Feature: professional-web-ui, Property 1: Active navigation item matches current route
		 * Validates: Requirements 2.5
		 */
		it('should mark dashboard as active for root and dashboard paths', () => {
			fc.assert(
				fc.property(
					fc.constantFrom('/', '/dashboard', '/dashboard/'),
					(pathname) => {
						const result = isActive('/dashboard', pathname);
						expect(result).toBe(true);
					}
				),
				{ numRuns: 100 }
			);
		});

		/**
		 * Feature: professional-web-ui, Property 1: Active navigation item matches current route
		 * Validates: Requirements 2.5
		 */
		it('should mark apps as active for any /apps/* path', () => {
			fc.assert(
				fc.property(
					fc.stringMatching(/^\/apps(\/[a-z0-9-]*)*$/),
					(pathname) => {
						const result = isActive('/apps', pathname);
						expect(result).toBe(true);
					}
				),
				{ numRuns: 100 }
			);
		});

		/**
		 * Feature: professional-web-ui, Property 1: Active navigation item matches current route
		 * Validates: Requirements 2.5
		 */
		it('should mark nodes as active for any /nodes/* path', () => {
			fc.assert(
				fc.property(
					fc.stringMatching(/^\/nodes(\/[a-z0-9-]*)*$/),
					(pathname) => {
						const result = isActive('/nodes', pathname);
						expect(result).toBe(true);
					}
				),
				{ numRuns: 100 }
			);
		});

		/**
		 * Feature: professional-web-ui, Property 1: Active navigation item matches current route
		 * Validates: Requirements 2.5
		 * 
		 * For any pathname, exactly one navigation item should be active (mutual exclusivity)
		 */
		it('should have at most one active navigation item for any valid route', () => {
			// Generate valid application routes
			const validRouteArb = fc.oneof(
				fc.constant('/'),
				fc.constant('/dashboard'),
				fc.stringMatching(/^\/apps(\/[a-z0-9-]+)*$/),
				fc.stringMatching(/^\/nodes(\/[a-z0-9-]+)*$/),
				fc.constant('/login'),
				fc.constant('/register')
			);

			fc.assert(
				fc.property(validRouteArb, (pathname) => {
					const activeItems = navItems.filter(item => isActive(item.href, pathname));
					// At most one item should be active
					expect(activeItems.length).toBeLessThanOrEqual(1);
				}),
				{ numRuns: 100 }
			);
		});

		/**
		 * Feature: professional-web-ui, Property 1: Active navigation item matches current route
		 * Validates: Requirements 2.5
		 * 
		 * Non-matching routes should not activate navigation items
		 */
		it('should not mark items as active for unrelated paths', () => {
			fc.assert(
				fc.property(
					fc.constantFrom('/login', '/register', '/settings', '/profile', '/unknown'),
					(pathname) => {
						// Dashboard should not be active for these paths
						expect(isActive('/dashboard', pathname)).toBe(false);
						// Apps should not be active for these paths
						expect(isActive('/apps', pathname)).toBe(false);
						// Nodes should not be active for these paths
						expect(isActive('/nodes', pathname)).toBe(false);
					}
				),
				{ numRuns: 100 }
			);
		});

		/**
		 * Feature: professional-web-ui, Property 1: Active navigation item matches current route
		 * Validates: Requirements 2.5
		 * 
		 * Prefix matching should work correctly - /apps should match /apps/123 but not /applications
		 */
		it('should use prefix matching correctly', () => {
			// /apps should match /apps/anything
			expect(isActive('/apps', '/apps/my-app')).toBe(true);
			expect(isActive('/apps', '/apps/my-app/services')).toBe(true);
			
			// /apps should NOT match /applications (different path)
			expect(isActive('/apps', '/applications')).toBe(false);
			
			// /nodes should match /nodes/anything
			expect(isActive('/nodes', '/nodes/node-1')).toBe(true);
			
			// /nodes should NOT match /nodeset
			expect(isActive('/nodes', '/nodeset')).toBe(false);
		});
	});
});
