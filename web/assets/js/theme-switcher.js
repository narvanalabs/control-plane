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

        // If View Transitions API is not available or no event to track coordinates
        if (!document.startViewTransition || !event) {
            updateTheme();
            return;
        }

        // Circular Reveal Effect
        const x = event.clientX ?? window.innerWidth / 2;
        const y = event.clientY ?? window.innerHeight / 2;
        const endRadius = Math.hypot(
            Math.max(x, window.innerWidth - x),
            Math.max(y, window.innerHeight - y)
        );

        const transition = document.startViewTransition(updateTheme);

        await transition.ready;

        document.documentElement.animate(
            {
                clipPath: [
                    `circle(0px at ${x}px ${y}px)`,
                    `circle(${endRadius}px at ${x}px ${y}px)`,
                ],
            },
            {
                duration: 500,
                easing: 'ease-in-out',
                pseudoElement: '::view-transition-new(root)',
            }
        );
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
