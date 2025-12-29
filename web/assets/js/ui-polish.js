// UI Polish for Narvana Control Plane
// Confirmation dialogs and button loading states

(function () {
    'use strict';

    document.addEventListener('DOMContentLoaded', function () {
        initConfirmations();
        initLoadingStates();
    });

    // Simple confirmation dialogs for destructive actions
    function initConfirmations() {
        document.addEventListener('click', function (e) {
            const target = e.target.closest('[data-confirm]');
            if (target) {
                const message = target.getAttribute('data-confirm') || 'Are you sure?';
                if (!confirm(message)) {
                    e.preventDefault();
                    e.stopPropagation();
                }
            }
        });
    }

    // Button loading states on form submission
    function initLoadingStates() {
        document.addEventListener('submit', function (e) {
            const form = e.target;
            if (form.method.toLowerCase() === 'post') {
                const submitBtn = form.querySelector('button[type="submit"]');
                if (submitBtn && !submitBtn.hasAttribute('data-no-loader')) {
                    // Disable button and add loading class/text
                    submitBtn.disabled = true;
                    const originalContent = submitBtn.innerHTML;
                    submitBtn.setAttribute('data-original-content', originalContent);

                    // Add spinner or change text
                    submitBtn.innerHTML = `
            <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-current inline" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            Processing...
          `;
                }
            }
        });

        // Reset buttons when page is shown from cache (back button)
        window.addEventListener('pageshow', function (event) {
            if (event.persisted) {
                document.querySelectorAll('button[data-original-content]').forEach(btn => {
                    btn.disabled = false;
                    btn.innerHTML = btn.getAttribute('data-original-content');
                    btn.removeAttribute('data-original-content');
                });
            }
        });
    }
})();
