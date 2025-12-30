(function () {
    'use strict';

    function initCheckboxes() {
        document.addEventListener('click', function (e) {
            const button = e.target.closest('[data-tui-checkbox]');
            if (!button || button.disabled) return;

            const isChecked = button.getAttribute('aria-checked') === 'true';
            const nextChecked = !isChecked;

            // Update button state
            button.setAttribute('aria-checked', nextChecked ? 'true' : 'false');
            button.setAttribute('data-state', nextChecked ? 'checked' : 'unchecked');

            // Update indicator
            const indicator = button.querySelector('[data-state]');
            if (indicator) {
                indicator.setAttribute('data-state', nextChecked ? 'checked' : 'unchecked');
            }

            // Update target input
            const id = button.id;
            const input = document.querySelector(`input[data-tui-checkbox-input="${id}"]`);
            if (input) {
                input.checked = nextChecked;
                // Dispatch change event on the hidden input
                input.dispatchEvent(new Event('change', { bubbles: true }));
            }
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initCheckboxes);
    } else {
        initCheckboxes();
    }
})();
