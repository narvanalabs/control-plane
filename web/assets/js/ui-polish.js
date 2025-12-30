// UI Polish for Narvana Control Plane
// Confirmation dialogs and button loading states

(function () {
    'use strict';

    document.addEventListener('DOMContentLoaded', function () {
        initConfirmations();
        initLoadingStates();
        initGitIntegration();
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

    // Real Git integration (currently GitHub) and Repo Picker
    async function initGitIntegration() {
        const gitSection = document.getElementById('git-connection-section');
        if (!gitSection) {
            // Check if we are in the detail page with a repo picker
            const repoPicker = document.getElementById('github-repo-picker');
            if (repoPicker) {
                loadGithubRepos();
            }
            return;
        }

        console.log('Initializing Git integration logic...');

        // Tab Switching
        const tabGithubApp = document.getElementById('tab-github-app');
        const tabOauthApp = document.getElementById('tab-oauth-app');
        const panelGithubApp = document.getElementById('panel-github-app');
        const panelOauthApp = document.getElementById('panel-oauth-app');

        if (tabGithubApp && tabOauthApp) {
            tabGithubApp.addEventListener('click', () => {
                tabGithubApp.classList.add('bg-zinc-800', 'text-white');
                tabGithubApp.classList.remove('text-muted-foreground');
                tabOauthApp.classList.remove('bg-zinc-800', 'text-white');
                tabOauthApp.classList.add('text-muted-foreground');
                panelGithubApp.classList.remove('hidden');
                panelOauthApp.classList.add('hidden');
            });

            tabOauthApp.addEventListener('click', () => {
                tabOauthApp.classList.add('bg-zinc-800', 'text-white');
                tabOauthApp.classList.remove('text-muted-foreground');
                tabGithubApp.classList.remove('bg-zinc-800', 'text-white');
                tabGithubApp.classList.add('text-muted-foreground');
                panelOauthApp.classList.remove('hidden');
                panelGithubApp.classList.add('hidden');
            });
        }

        // Organization Toggle Logic (TemplUI Checkbox)
        const orgInputGroup = document.getElementById('git-org-input-group');
        const actualOrgInput = document.querySelector('input[data-tui-checkbox-input="git-is-org"]');
        if (actualOrgInput && orgInputGroup) {
            actualOrgInput.addEventListener('change', function () {
                if (actualOrgInput.checked) {
                    orgInputGroup.classList.remove('hidden');
                } else {
                    orgInputGroup.classList.add('hidden');
                }
            });
        }

        // GitHub App Manifest Flow
        const btnConnectManifest = document.getElementById('btn-connect-github-manifest');
        if (btnConnectManifest) {
            btnConnectManifest.addEventListener('click', async () => {
                const appNameInput = document.getElementById('git-app-name');
                const orgNameInput = document.getElementById('git-org-name');
                const appName = appNameInput?.value || '';
                const isOrg = actualOrgInput?.checked || false;
                const orgName = orgNameInput?.value || '';

                btnConnectManifest.disabled = true;
                btnConnectManifest.innerHTML = '<span class="animate-pulse mr-2">⏳</span> Redirecting...';

                try {
                    let url = `/api/github/setup?app_name=${encodeURIComponent(appName)}`;
                    if (isOrg && orgName) {
                        url += `&org=${encodeURIComponent(orgName)}`;
                    }

                    // Directly navigate to the endpoint that serves the auto-submitting POST form
                    window.location.href = url;
                } catch (err) {
                    console.error('Setup failed:', err);
                    alert('Error: ' + err.message);
                    btnConnectManifest.disabled = false;
                    btnConnectManifest.innerHTML = 'Create & Connect GitHub App';
                }
            });
        }

        // OAuth Manual Save Flow
        const btnSaveOauth = document.getElementById('btn-save-oauth-config');
        if (btnSaveOauth) {
            btnSaveOauth.addEventListener('click', async () => {
                const clientId = document.getElementById('git-oauth-client-id')?.value || '';
                const clientSecret = document.getElementById('git-oauth-client-secret')?.value || '';

                if (!clientId || !clientSecret) {
                    alert('Client ID and Client Secret are required.');
                    return;
                }

                btnSaveOauth.disabled = true;
                btnSaveOauth.innerHTML = '<span class="animate-pulse mr-2">⏳</span> Saving...';

                try {
                    const response = await fetch('/api/github/config', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            config_type: 'oauth',
                            client_id: clientId,
                            client_secret: clientSecret
                        })
                    });

                    const data = await response.json();
                    if (data.status === 'success') {
                        window.location.reload();
                    } else {
                        throw new Error(data.error || 'Failed to save configuration');
                    }
                } catch (err) {
                    console.error('Save failed:', err);
                    alert('Error: ' + err.message);
                    btnSaveOauth.disabled = false;
                    btnSaveOauth.innerHTML = 'Save OAuth Configuration';
                }
            });
        }

        // Instance Connection (Installation or OAuth Login)
        const btnConnectInstance = document.getElementById('btn-connect-github-instance');
        if (btnConnectInstance) {
            btnConnectInstance.addEventListener('click', async () => {
                btnConnectInstance.disabled = true;
                btnConnectInstance.innerHTML = '<span class="animate-pulse mr-2">⏳</span> Connecting...';

                try {
                    const response = await fetch('/api/github/connect');
                    const data = await response.json();

                    if (data.url) {
                        window.location.href = data.url;
                    } else {
                        throw new Error(data.error || 'Failed to connect');
                    }
                } catch (err) {
                    console.error('Connection failed:', err);
                    alert('Error: ' + err.message);
                    btnConnectInstance.disabled = false;
                    btnConnectInstance.innerHTML = 'Connect';
                }
            });
        }

        // Reconfigure logic
        const btnReconfigure = document.getElementById('btn-reconfigure-github');
        if (btnReconfigure) {
            btnReconfigure.addEventListener('click', async () => {
                if (confirm('Reconfiguring will disconnect your current GitHub connection and clear all installations. Are you sure?')) {
                    btnReconfigure.disabled = true;
                    btnReconfigure.innerHTML = '<span class="animate-pulse mr-2">⏳</span> Resetting...';

                    try {
                        const response = await fetch('/api/github/config', {
                            method: 'DELETE'
                        });
                        const data = await response.json();
                        if (data.status === 'success') {
                            window.location.reload();
                        } else {
                            throw new Error(data.error || 'Failed to reset configuration');
                        }
                    } catch (err) {
                        console.error('Reset failed:', err);
                        alert('Error: ' + err.message);
                        btnReconfigure.disabled = false;
                        btnReconfigure.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-4 w-4 mr-2"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.1a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"></path><circle cx="12" cy="12" r="3"></circle></svg>Reconfigure';
                    }
                }
            });
        }

        async function loadGithubRepos() {
            const repoPicker = document.getElementById('github-repo-picker');
            const manualInput = document.getElementById('manual-repo-input');
            const hiddenRepo = document.getElementById('selected-github-repo');
            if (!repoPicker) return;

            try {
                const resp = await fetch('/api/github/repos');
                if (!resp.ok) {
                    const contentZone = document.querySelector('#github_repo_select [data-tui-selectbox-content]');
                    if (contentZone) contentZone.innerHTML = '<div class="p-4 text-sm text-destructive text-center">Failed to load repositories</div>';
                    return;
                }
                const repos = await resp.json();

                const contentZone = document.querySelector('#github_repo_select [data-tui-selectbox-content]');
                if (!contentZone) return;

                if (repos && repos.length > 0) {
                    repoPicker.classList.remove('hidden');
                    if (manualInput) manualInput.classList.add('opacity-50');

                    contentZone.innerHTML = '';
                    repos.forEach(repo => {
                        const item = document.createElement('div');
                        item.className = 'relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none hover:bg-accent hover:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50';
                        item.setAttribute('data-tui-selectbox-item', 'true');
                        item.setAttribute('data-value', repo.full_name);
                        item.innerHTML = `
                            <span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center opacity-0" data-tui-selectbox-item-indicator="true">
                                <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-4 w-4"><polyline points="20 6 9 17 4 12"></polyline></svg>
                            </span>
                            <span class="flex flex-col">
                                <span class="font-medium text-white">${repo.full_name}</span>
                                <span class="text-[11px] text-muted-foreground truncate max-w-[200px]">${repo.description || 'No description'}</span>
                            </span>
                        `;
                        contentZone.appendChild(item);
                    });
                } else {
                    contentZone.innerHTML = '<div class="p-4 text-sm text-muted-foreground text-center">No repositories found</div>';
                }
            } catch (err) {
                console.error('Error loading GitHub repos:', err);
                const contentZone = document.querySelector('#github_repo_select [data-tui-selectbox-content]');
                if (contentZone) contentZone.innerHTML = '<div class="p-4 text-sm text-destructive text-center">Error loading repositories</div>';
            }
        }

        // Listen for repo selection
        document.addEventListener('change', function (e) {
            const target = e.target;
            if (target && target.hasAttribute('data-tui-selectbox-hidden-input')) {
                const selectContainer = target.closest('#github_repo_select');
                const hiddenRepo = document.getElementById('selected-github-repo');
                if (selectContainer && hiddenRepo) {
                    hiddenRepo.value = target.value;
                    const manualInputEl = document.getElementById('svc_repo');
                    if (manualInputEl) manualInputEl.value = target.value;
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
