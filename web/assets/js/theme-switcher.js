/**
 * Theme Switcher for Narvana Control Plane
 * Handles dark/light mode toggling and persistence.
 */
(function () {
    'use strict';

    const THEME_KEY = 'narvana-theme';

    function initTheme() {
        const savedTheme = localStorage.getItem(THEME_KEY);
        const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;

        const isDark = savedTheme === 'dark' || (!savedTheme && systemPrefersDark);

        if (isDark) {
            document.documentElement.classList.add('dark');
        } else {
            document.documentElement.classList.remove('dark');
        }

        // Sync any theme switches on the page
        syncSwitches(isDark);
    }

    function syncSwitches(isDark) {
        const switches = document.querySelectorAll('[data-theme-switcher]');
        switches.forEach(sw => {
            const hiddenInput = document.querySelector(`[data-tui-switch-input="${sw.id}"]`);
            if (hiddenInput) {
                hiddenInput.checked = isDark;
                // Trigger change to update switch UI
                hiddenInput.dispatchEvent(new Event('change', { bubbles: true }));
            }
        });
    }

    async function toggleTheme(isDark, event) {
        const theme = isDark ? 'dark' : 'light';
        localStorage.setItem(THEME_KEY, theme);

        const updateTheme = () => {
            if (isDark) {
                document.documentElement.classList.add('dark');
            } else {
                document.documentElement.classList.remove('dark');
            }

            // Sync all other switches on the page
            const switches = document.querySelectorAll('[data-theme-switcher]');
            switches.forEach(sw => {
                const hiddenInput = document.querySelector(`[data-tui-switch-input="${sw.id}"]`);
                if (hiddenInput && hiddenInput.checked !== isDark) {
                    hiddenInput.checked = isDark;
                    hiddenInput.dispatchEvent(new Event('change', { bubbles: true }));
                }
            });
        };

        // If View Transitions API is not available
        if (!document.startViewTransition) {
            updateTheme();
            return;
        }

        // Sleek Synchronized Cross-fade
        document.documentElement.setAttribute('data-view-transitioning', 'true');

        const transition = document.startViewTransition(updateTheme);

        try {
            await transition.finished;
        } finally {
            document.documentElement.removeAttribute('data-view-transitioning');
        }
    }

    document.addEventListener('DOMContentLoaded', function () {
        initTheme();

        // Listen for changes from theme switches
        document.addEventListener('change', function (e) {
            const target = e.target;
            if (target && target.hasAttribute('data-theme-switcher-input')) {
                // Try to find the original click event if possible or use the change event
                toggleTheme(target.checked, e);
            }
        });
    });

    // Handle system preference changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
        if (!localStorage.getItem(THEME_KEY)) {
            toggleTheme(e.matches);
        }
    });

})();
