/**
 * Theme Switcher for Narvana Control Plane
 * Handles tri-state (light, dark, system) toggling and persistence.
 */
(function () {
    'use strict';

    const THEME_KEY = 'narvana-theme';

    function getTheme() {
        return localStorage.getItem(THEME_KEY) || 'system';
    }

    function applyTheme(theme) {
        const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);

        if (isDark) {
            document.documentElement.classList.add('dark');
        } else {
            document.documentElement.classList.remove('dark');
        }

        document.documentElement.setAttribute('data-theme-state', theme);

        // Sync all tri-state controls on the page
        syncControls(theme);
    }

    function syncControls(theme) {
        // Find all elements that want to show active state based on theme
        const triggers = document.querySelectorAll('[data-theme-value]');
        triggers.forEach(trig => {
            const val = trig.getAttribute('data-theme-value');
            if (val === theme) {
                trig.setAttribute('data-state', 'active');
                trig.setAttribute('data-tui-tabs-state', 'active');
            } else {
                trig.setAttribute('data-state', 'inactive');
                trig.setAttribute('data-tui-tabs-state', 'inactive');
            }
        });

        // Sync cycle buttons (those that show current theme icon)
        const cycleButtons = document.querySelectorAll('[data-theme-cycle]');
        cycleButtons.forEach(btn => {
            const icons = btn.querySelectorAll('[data-theme-icon]');
            icons.forEach(icon => {
                if (icon.getAttribute('data-theme-icon') === theme) {
                    icon.classList.remove('hidden');
                } else {
                    icon.classList.add('hidden');
                }
            });
        });

        // Backward compatibility for any remaining data-labels
        const labels = document.querySelectorAll('[data-theme-label]');
        const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
        labels.forEach(lb => {
            lb.textContent = isDark ? 'Dark Mode' : 'Light Mode';
        });

        // Compatibility for any binary switches
        const switches = document.querySelectorAll('[data-theme-switcher-input]');
        switches.forEach(sw => {
            if (sw.checked !== isDark) {
                sw.checked = isDark;
                sw.dispatchEvent(new Event('change', { bubbles: true }));
            }
        });
    }

    function getNextTheme(current) {
        if (current === 'light') return 'dark';
        if (current === 'dark') return 'system';
        return 'light';
    }

    async function setTheme(theme) {
        const current = getTheme();
        if (current === theme) return;

        localStorage.setItem(THEME_KEY, theme);

        const updateTheme = () => applyTheme(theme);

        if (!document.startViewTransition) {
            updateTheme();
            return;
        }

        document.documentElement.setAttribute('data-view-transitioning', 'true');
        const transition = document.startViewTransition(updateTheme);

        try {
            await transition.finished;
        } finally {
            document.documentElement.removeAttribute('data-view-transitioning');
        }
    }

    async function cycleTheme() {
        const current = getTheme();
        const next = getNextTheme(current);
        await setTheme(next);
    }

    function initTheme() {
        applyTheme(getTheme());
    }

    document.addEventListener('DOMContentLoaded', function () {
        initTheme();

        // Listen for clicks
        document.addEventListener('click', function (e) {
            // Tri-state fixed selection
            const trigger = e.target.closest('[data-theme-value]');
            if (trigger) {
                const theme = trigger.getAttribute('data-theme-value');
                setTheme(theme);
                return;
            }

            // Tri-state cycle button
            const cycler = e.target.closest('[data-theme-cycle]');
            if (cycler) {
                cycleTheme();
                return;
            }
        });

        // Handle binary switches if any remain
        document.addEventListener('change', function (e) {
            const target = e.target;
            if (target && target.hasAttribute('data-theme-switcher-input')) {
                const newTheme = target.checked ? 'dark' : 'light';
                if (getTheme() !== newTheme) {
                    setTheme(newTheme);
                }
            }
        });
    });

    // Handle system preference changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (getTheme() === 'system') {
            applyTheme('system');
        }
    });

})();
