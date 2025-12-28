// Global state stores using Svelte 5 runes
import { auth, type User } from './api';

// Auth state
class AuthState {
	isAuthenticated = $state(false);
	user = $state<User | null>(null);
	isLoading = $state(true);

	constructor() {
		this.init();
	}

	private init() {
		if (typeof window === 'undefined') return;
		
		this.isAuthenticated = auth.isAuthenticated();
		this.user = auth.getUser();
		this.isLoading = false;
	}

	login(user: User) {
		this.isAuthenticated = true;
		this.user = user;
	}

	logout() {
		auth.logout();
		this.isAuthenticated = false;
		this.user = null;
	}

	async validate() {
		try {
			const result = await auth.validate();
			this.isAuthenticated = true;
			return result;
		} catch {
			this.logout();
			return null;
		}
	}
}

export const authState = new AuthState();

// Theme state
class ThemeState {
	isDark = $state(true);

	constructor() {
		if (typeof window !== 'undefined') {
			const stored = localStorage.getItem('narvana_theme');
			this.isDark = stored ? stored === 'dark' : true;
		}
	}

	toggle() {
		this.isDark = !this.isDark;
		if (typeof window !== 'undefined') {
			localStorage.setItem('narvana_theme', this.isDark ? 'dark' : 'light');
		}
	}
}

export const themeState = new ThemeState();



