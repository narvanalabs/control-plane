// UI Polish for Narvana Control Plane
// Confirmation dialogs and button loading states

(function () {
    'use strict';

    document.addEventListener('DOMContentLoaded', function () {
        initConfirmations();
        initLoadingStates();
        initGithubMockup();
        initCategorySelection();
    });

    // Handle auto-selecting category from dropdown items and triggering dialog
    function initCategorySelection() {
        document.addEventListener('click', function (e) {
            const target = e.target.closest('[data-set-category]');
            if (!target) return;

            const category = target.getAttribute('data-set-category');
            const dialogId = target.getAttribute('data-tui-dialog-trigger');

            console.log('Category item clicked:', category, 'Dialog:', dialogId);

            // 1. Manually trigger the dialog if it exists
            if (dialogId) {
                const dialogContent = document.querySelector(`[data-tui-dialog-content][data-dialog-instance="${dialogId}"]`);
                const backdrop = document.querySelector(`[data-tui-dialog-backdrop][data-dialog-instance="${dialogId}"]`);

                if (dialogContent && backdrop) {
                    dialogContent.setAttribute('data-tui-dialog-open', 'true');
                    dialogContent.removeAttribute('data-tui-dialog-hidden');
                    backdrop.setAttribute('data-tui-dialog-open', 'true');
                    backdrop.removeAttribute('data-tui-dialog-hidden');

                    // Dispatch a custom event to let templui know the dialog is open if it doesn't auto-react
                    document.dispatchEvent(new CustomEvent('tui-dialog:open', { detail: { id: dialogId } }));
                    console.log(`Dialog '${dialogId}' manually opened.`);
                } else {
                    console.warn(`Could not find dialog content or backdrop for dialog ID: ${dialogId}`);
                }
            }

            // 2. Set the category in the hidden input and indicator
            // Give it a tiny bit of time for the dialog to transition/render if needed
            setTimeout(() => {
                const hiddenInput = document.getElementById('hidden_svc_category');
                const indicator = document.getElementById('svc_category_indicator');

                if (hiddenInput) {
                    hiddenInput.value = category;
                    console.log('Category hidden field set to:', category);
                }

                if (indicator) {
                    indicator.innerText = category.replace('-', ' ');
                }
            }, 50);
        });
    }

    // Mockup for GitHub connection and Repo Picker
    function initGithubMockup() {
        const btnConnect = document.getElementById('btn-connect-github');
        const repoPicker = document.getElementById('github-repo-picker');
        const manualInput = document.getElementById('manual-repo-input');
        const hiddenRepo = document.getElementById('selected-github-repo');

        if (!btnConnect) return;

        btnConnect.addEventListener('click', function () {
            btnConnect.disabled = true;
            btnConnect.innerHTML = '<span class="animate-pulse">Connecting...</span>';

            // Simulate API delay
            setTimeout(() => {
                btnConnect.classList.add('hidden');
                repoPicker.classList.remove('hidden');
                manualInput.classList.add('opacity-50');

                // Show a success toast (simulated)
                if (window.showToast) {
                    window.showToast('Successfully connected to GitHub', 'success');
                }
            }, 800);
        });

        // Listen for selection changes on the repo picker
        document.addEventListener('change', function (e) {
            const target = e.target;
            if (target && target.hasAttribute('data-tui-selectbox-hidden-input')) {
                // Check if this input belongs to the github_repo_select component
                const selectContainer = target.closest('#github_repo_select');
                if (selectContainer && hiddenRepo) {
                    hiddenRepo.value = target.value;
                    console.log('Selected GitHub Repo:', target.value);
                }
            }
        });
    }

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
