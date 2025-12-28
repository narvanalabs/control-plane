/**
 * Navigation utilities for Sidebar component
 * Requirements: 2.5
 */

/**
 * Determines if a navigation item is active based on current route.
 * Property 1: Active navigation item matches current route
 * 
 * Uses proper path segment matching to avoid false positives like
 * /nodes matching /nodeset. A path matches if it equals the href
 * or starts with href followed by a slash.
 * 
 * @param href - The navigation item's href
 * @param pathname - The current route pathname
 * @returns true if the navigation item should be marked as active
 */
export function isActive(href: string, pathname: string): boolean {
	// Normalize pathname by removing trailing slash (except for root)
	const normalizedPath = pathname.length > 1 && pathname.endsWith('/') 
		? pathname.slice(0, -1) 
		: pathname;
	
	if (href === '/dashboard') {
		return normalizedPath === '/dashboard' || normalizedPath === '/';
	}
	
	// Check for exact match or path segment match (href followed by /)
	return normalizedPath === href || normalizedPath.startsWith(href + '/');
}
