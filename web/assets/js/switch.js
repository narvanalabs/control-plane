/**
 * Switch component for templui.
 * Handles state transitions and synchronization with a hidden checkbox input.
 */
(function () {
    'use strict';

    function initSwitch(el) {
        if (el.hasAttribute('data-tui-switch-initialized')) return;
        el.setAttribute('data-tui-switch-initialized', 'true');

        const switchId = el.id;
        const hiddenInput = document.querySelector(`[data-tui-switch-input="${switchId}"]`);

        el.addEventListener('click', function (e) {
            if (el.disabled) return;

            e.preventDefault();
            const isChecked = el.getAttribute('aria-checked') === 'true';
            const newState = !isChecked;

            updateSwitchState(el, hiddenInput, newState);

            // Dispatch change event on the hidden input
            if (hiddenInput) {
                hiddenInput.dispatchEvent(new Event('change', { bubbles: true }));
            }
        });

        // Handle external changes to the hidden input
        if (hiddenInput) {
            hiddenInput.addEventListener('change', function () {
                const isChecked = hiddenInput.checked;
                updateSwitchState(el, null, isChecked);
            });
        }
    }

    function updateSwitchState(el, hiddenInput, isChecked) {
        const stateStr = isChecked ? 'checked' : 'unchecked';
        el.setAttribute('aria-checked', isChecked ? 'true' : 'false');
        el.setAttribute('data-state', stateStr);

        const thumb = el.querySelector('span');
        if (thumb) {
            thumb.setAttribute('data-state', stateStr);
            // Update icons inside the thumb
            thumb.querySelectorAll('[data-state]').forEach(child => {
                child.setAttribute('data-state', stateStr);
            });
        }

        if (hiddenInput) {
            hiddenInput.checked = isChecked;
        }
    }

    function scan() {
        document.querySelectorAll('[data-tui-switch]').forEach(initSwitch);
    }

    // Initialize on DOM load and subsequent updates
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', scan);
    } else {
        scan();
    }

    // Listen for htmx:afterSwap to re-initialize
    document.addEventListener('htmx:afterSwap', function () {
        scan();
    });
})();
