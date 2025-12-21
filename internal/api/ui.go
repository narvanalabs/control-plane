package api

const authUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Narvana - Setup</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #fff;
        }
        .container {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 40px;
            width: 100%;
            max-width: 400px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        h1 { font-size: 28px; margin-bottom: 8px; }
        .subtitle { color: #888; margin-bottom: 32px; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 8px; font-size: 14px; color: #aaa; }
        input {
            width: 100%;
            padding: 12px 16px;
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            background: rgba(255,255,255,0.05);
            color: #fff;
            font-size: 16px;
        }
        input:focus { outline: none; border-color: #6366f1; }
        button {
            width: 100%;
            padding: 14px;
            border: none;
            border-radius: 8px;
            background: #6366f1;
            color: #fff;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.2s;
        }
        button:hover { background: #5558e3; }
        button:disabled { background: #444; cursor: not-allowed; }
        .error { color: #ef4444; margin-top: 16px; font-size: 14px; }
        .success { color: #22c55e; margin-top: 16px; font-size: 14px; }
        .toggle { text-align: center; margin-top: 24px; }
        .toggle a { color: #6366f1; cursor: pointer; text-decoration: none; }
        .toggle a:hover { text-decoration: underline; }
        .setup-msg { background: rgba(99,102,241,0.2); padding: 16px; border-radius: 8px; margin-bottom: 24px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 Narvana</h1>
        <p class="subtitle" id="subtitle">Loading...</p>
        
        <div id="setup-msg" class="setup-msg" style="display:none;">
            Welcome! Create your admin account to get started.
        </div>
        
        <form id="auth-form">
            <div class="form-group">
                <label for="email">Email</label>
                <input type="email" id="email" required placeholder="admin@example.com">
            </div>
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" required placeholder="Min 8 characters" minlength="8">
            </div>
            <button type="submit" id="submit-btn">Continue</button>
        </form>
        
        <div id="error" class="error"></div>
        <div id="success" class="success"></div>
        
        <div class="toggle" id="toggle-container" style="display:none;">
            <a id="toggle-link">Switch to login</a>
        </div>
    </div>

    <script>
        let isRegister = false;
        let setupComplete = false;
        
        // Check for redirect parameter (from device auth page)
        const params = new URLSearchParams(window.location.search);
        const redirectCode = params.get('redirect');
        
        async function checkSetup() {
            const res = await fetch('/auth/setup');
            const data = await res.json();
            setupComplete = data.setup_complete;
            
            if (!setupComplete) {
                isRegister = true;
                document.getElementById('subtitle').textContent = 'Initial Setup';
                document.getElementById('setup-msg').style.display = 'block';
                document.getElementById('submit-btn').textContent = 'Create Admin Account';
            } else {
                document.getElementById('subtitle').textContent = 'Sign in to continue';
                document.getElementById('toggle-container').style.display = 'block';
                document.getElementById('submit-btn').textContent = 'Sign In';
            }
        }
        
        document.getElementById('toggle-link').addEventListener('click', () => {
            isRegister = !isRegister;
            document.getElementById('toggle-link').textContent = isRegister ? 'Switch to login' : 'Create account';
            document.getElementById('submit-btn').textContent = isRegister ? 'Create Account' : 'Sign In';
            document.getElementById('error').textContent = '';
        });
        
        document.getElementById('auth-form').addEventListener('submit', async (e) => {
            e.preventDefault();
            const email = document.getElementById('email').value;
            const password = document.getElementById('password').value;
            const btn = document.getElementById('submit-btn');
            
            btn.disabled = true;
            document.getElementById('error').textContent = '';
            
            try {
                const endpoint = isRegister ? '/auth/register' : '/auth/login';
                const res = await fetch(endpoint, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ email, password })
                });
                
                const data = await res.json();
                
                if (!res.ok) {
                    throw new Error(data.error || 'Authentication failed');
                }
                
                // Store token
                localStorage.setItem('narvana_token', data.token);
                localStorage.setItem('narvana_user', JSON.stringify({ id: data.user_id, email: data.email }));
                
                document.getElementById('success').textContent = 'Success! Redirecting...';
                
                // Redirect to device auth if we came from there
                if (redirectCode) {
                    window.location.href = '/auth/device?code=' + redirectCode;
                } else {
                    document.getElementById('success').textContent = '✓ Logged in successfully!';
                }
            } catch (err) {
                document.getElementById('error').textContent = err.message;
            } finally {
                btn.disabled = false;
            }
        });
        
        checkSetup();
    </script>
</body>
</html>`

const deviceAuthUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Narvana - Authorize Device</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #fff;
        }
        .container {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 40px;
            width: 100%;
            max-width: 400px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
            text-align: center;
        }
        h1 { font-size: 28px; margin-bottom: 8px; }
        .subtitle { color: #888; margin-bottom: 32px; }
        .code-display {
            background: rgba(99,102,241,0.2);
            padding: 24px;
            border-radius: 12px;
            margin-bottom: 24px;
        }
        .code {
            font-size: 32px;
            font-family: monospace;
            letter-spacing: 4px;
            color: #6366f1;
        }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 8px; font-size: 14px; color: #aaa; }
        input {
            width: 100%;
            padding: 12px 16px;
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            background: rgba(255,255,255,0.05);
            color: #fff;
            font-size: 16px;
            text-align: center;
            letter-spacing: 2px;
        }
        input:focus { outline: none; border-color: #6366f1; }
        button {
            width: 100%;
            padding: 14px;
            border: none;
            border-radius: 8px;
            background: #6366f1;
            color: #fff;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.2s;
        }
        button:hover { background: #5558e3; }
        button:disabled { background: #444; cursor: not-allowed; }
        .error { color: #ef4444; margin-top: 16px; font-size: 14px; }
        .success { color: #22c55e; margin-top: 16px; font-size: 14px; }
        .login-link { margin-top: 24px; }
        .login-link a { color: #6366f1; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 Authorize CLI</h1>
        <p class="subtitle" id="subtitle">Confirm the code to authorize your CLI</p>
        
        <div class="code-display" id="code-display" style="display:none;">
            <div class="code" id="display-code"></div>
        </div>
        
        <form id="approve-form">
            <button type="submit" id="approve-btn">Authorize This Device</button>
        </form>
        
        <div id="error" class="error"></div>
        <div id="success" class="success"></div>
    </div>

    <script>
        const token = localStorage.getItem('narvana_token');
        const params = new URLSearchParams(window.location.search);
        const code = params.get('code');
        
        // If no token, redirect to login with return URL
        if (!token) {
            window.location.href = '/?redirect=' + (code || '');
        }
        
        // Show the code
        if (code) {
            document.getElementById('code-display').style.display = 'block';
            document.getElementById('display-code').textContent = code.toUpperCase();
            document.getElementById('subtitle').textContent = 'Click to authorize this device';
        } else {
            document.getElementById('error').textContent = 'No device code provided';
            document.getElementById('approve-btn').disabled = true;
        }
        
        document.getElementById('approve-form').addEventListener('submit', async (e) => {
            e.preventDefault();
            const btn = document.getElementById('approve-btn');
            
            btn.disabled = true;
            btn.textContent = 'Authorizing...';
            document.getElementById('error').textContent = '';
            
            try {
                const res = await fetch('/auth/device/approve', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ user_code: code, token: token })
                });
                
                const data = await res.json();
                
                if (!res.ok) {
                    throw new Error(data.error || 'Authorization failed');
                }
                
                document.getElementById('success').textContent = '✓ Device authorized! You can close this window.';
                document.getElementById('approve-form').style.display = 'none';
                document.getElementById('code-display').style.display = 'none';
                document.getElementById('subtitle').textContent = 'Authorization complete';
            } catch (err) {
                document.getElementById('error').textContent = err.message;
                btn.disabled = false;
                btn.textContent = 'Authorize This Device';
            }
        });
    </script>
</body>
</html>`
