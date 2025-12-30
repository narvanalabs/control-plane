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

    function toggleTheme(isDark) {
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

        // Use View Transitions API if available
        if (document.startViewTransition) {
            document.startViewTransition(updateTheme);
        } else {
            updateTheme();
        }
    }

    document.addEventListener('DOMContentLoaded', function () {
        initTheme();

        // Listen for changes from theme switches
        document.addEventListener('change', function (e) {
            const target = e.target;
            if (target && target.hasAttribute('data-theme-switcher-input')) {
                toggleTheme(target.checked);
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
