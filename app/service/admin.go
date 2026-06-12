package service

import (
	"chat2api/app/conf"
	"chat2api/app/result"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func AdminPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(adminPageHTML))
}

func AdminLogin(c *gin.Context) {
	req := adminLoginRequest{}
	jb := result.New(c, "admin_login")
	if jb.BindJson(&req) {
		return
	}
	if !conf.AdminEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": gin.H{"msg": "admin login is not configured"}})
		return
	}
	if !conf.ValidateAdminCredentials(req.Username, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": gin.H{"msg": "invalid admin username or password"}})
		return
	}
	token, err := conf.NewAdminSessionToken()
	if jb.AssertError(err) {
		return
	}
	secureCookie := isSecureRequest(c)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(conf.AdminSessionCookieName(), token, 12*60*60, "/", "", secureCookie, true)
	jb.Data = gin.H{"logged_in": true, "username": conf.AdminUsername()}
	jb.Successful()
}

func AdminLogout(c *gin.Context) {
	conf.ClearAdminSessionToken()
	secureCookie := isSecureRequest(c)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(conf.AdminSessionCookieName(), "", -1, "/", "", secureCookie, true)
	result.New(c, "admin_logout").AssertSuccessful(gin.H{"logged_out": true}, nil)
}

func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

func AdminSession(c *gin.Context) {
	result.New(c, "admin_session").AssertSuccessful(gin.H{"logged_in": true, "username": conf.AdminUsername()}, nil)
}

func AdminGetConfig(c *gin.Context) {
	snapshot, err := conf.AdminSnapshot()
	result.New(c, "admin_get_config").Finish(snapshot, err)
}

func AdminSaveConfig(c *gin.Context) {
	req := conf.AdminConfigUpdate{}
	jb := result.New(c, "admin_save_config")
	if jb.BindJson(&req) {
		return
	}
	if err := conf.SaveAdminConfig(req); jb.AssertError(err) {
		return
	}
	jb.Data = gin.H{"saved": true}
	jb.Successful()
}

func AdminGenerateAuthToken(c *gin.Context) {
	token, err := conf.NewAuthToken()
	jb := result.New(c, "admin_generate_auth_token")
	if jb.AssertError(err) {
		return
	}
	jb.AssertSuccessful(gin.H{"token": token}, nil)
}

func AdminExportConfig(c *gin.Context) {
	filename, data, err := conf.ExportAdminConfig()
	if result.New(c, "admin_export_config").AssertError(err) {
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/x-yaml; charset=utf-8", data)
}

func AdminImportConfig(c *gin.Context) {
	file, err := c.FormFile("file")
	jb := result.New(c, "admin_import_config")
	if jb.AssertError(err) {
		return
	}
	stream, err := file.Open()
	if jb.AssertError(err) {
		return
	}
	defer stream.Close()
	data, err := io.ReadAll(stream)
	if jb.AssertError(err) {
		return
	}
	if err := conf.ImportAdminConfig(data); jb.AssertError(err) {
		return
	}
	jb.Data = gin.H{"imported": true}
	jb.Successful()
}

const adminPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>chat2api · 管理面板</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link href="https://fonts.googleapis.com/css2?family=Inter:opsz,wght@14..32,400;14..32,500;14..32,600;14..32,700;14..32,800&display=swap" rel="stylesheet">
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    :root {
      --bg: #0f0f13;
      --surface: rgba(255,255,255,0.04);
      --surface-hover: rgba(255,255,255,0.08);
      --surface-active: rgba(255,255,255,0.12);
      --border: rgba(255,255,255,0.06);
      --border-focus: rgba(139,92,246,0.5);
      --text: #f1f1f7;
      --text-secondary: rgba(255,255,255,0.55);
      --text-tertiary: rgba(255,255,255,0.35);
      --accent: #8b5cf6;
      --accent-hover: #a78bfa;
      --accent-glow: rgba(139,92,246,0.25);
      --success: #34d399;
      --success-bg: rgba(52,211,153,0.1);
      --warning: #fbbf24;
      --warning-bg: rgba(251,191,36,0.1);
      --danger: #f87171;
      --danger-bg: rgba(248,113,113,0.1);
      --radius: 12px;
      --radius-lg: 16px;
      --radius-xl: 20px;
      --shadow: 0 1px 3px rgba(0,0,0,0.3), 0 1px 2px rgba(0,0,0,0.2);
      --shadow-lg: 0 10px 40px rgba(0,0,0,0.4), 0 2px 8px rgba(0,0,0,0.3);
      --shadow-xl: 0 20px 60px rgba(0,0,0,0.5);
      --font: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
      --mono: 'JetBrains Mono', 'SF Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace;
      --transition: 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    }
    html { font-size: 15px; }
    body {
      font-family: var(--font);
      background: var(--bg);
      color: var(--text);
      min-height: 100vh;
      line-height: 1.6;
      -webkit-font-smoothing: antialiased;
      -moz-osx-font-smoothing: grayscale;
    }
    /* Scrollbar */
    ::-webkit-scrollbar { width: 6px; height: 6px; }
    ::-webkit-scrollbar-track { background: transparent; }
    ::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.1); border-radius: 3px; }
    ::-webkit-scrollbar-thumb:hover { background: rgba(255,255,255,0.2); }
    /* Layout */
    .app {
      max-width: 1360px;
      margin: 0 auto;
      padding: 24px 20px 48px;
    }
    /* Hero / Login */
    .hero {
      position: relative;
      overflow: hidden;
      border-radius: var(--radius-xl);
      background: linear-gradient(135deg, #1c1c2e 0%, #2a1f3d 40%, #1a1a2e 100%);
      border: 1px solid var(--border);
      box-shadow: var(--shadow-xl);
      padding: 40px;
      margin-bottom: 24px;
    }
    .hero::before {
      content: '';
      position: absolute;
      top: -50%;
      left: -50%;
      width: 200%;
      height: 200%;
      background: radial-gradient(circle at 30% 30%, rgba(139,92,246,0.08) 0%, transparent 50%),
                  radial-gradient(circle at 70% 70%, rgba(52,211,153,0.05) 0%, transparent 50%);
      pointer-events: none;
    }
    .hero-grid {
      display: grid;
      grid-template-columns: 1.3fr 1fr;
      gap: 32px;
      align-items: center;
      position: relative;
      z-index: 1;
    }
    .hero h1 {
      font-size: clamp(28px, 4vw, 42px);
      font-weight: 800;
      letter-spacing: -0.04em;
      line-height: 1.1;
      background: linear-gradient(135deg, #f1f1f7 0%, #c4b5fd 100%);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      background-clip: text;
      margin-bottom: 12px;
    }
    .hero .subtitle {
      color: var(--text-secondary);
      font-size: 0.95rem;
      line-height: 1.7;
      max-width: 52ch;
    }
    .login-card {
      background: rgba(255,255,255,0.03);
      border: 1px solid rgba(255,255,255,0.08);
      border-radius: var(--radius-lg);
      padding: 28px;
      backdrop-filter: blur(20px);
    }
    .login-card h2 {
      font-size: 0.8rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.1em;
      color: var(--text-secondary);
      margin-bottom: 6px;
    }
    .login-card > p {
      color: var(--text-tertiary);
      font-size: 0.85rem;
      margin-bottom: 20px;
    }
    .login-card .field {
      margin-bottom: 14px;
    }
    .login-card .field:last-of-type {
      margin-bottom: 18px;
    }
    /* Status badge */
    .status-badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 6px 14px;
      border-radius: 999px;
      font-size: 0.8rem;
      font-weight: 600;
      background: var(--surface);
      border: 1px solid var(--border);
      color: var(--text-secondary);
    }
    .status-badge.ok {
      background: var(--success-bg);
      border-color: rgba(52,211,153,0.2);
      color: var(--success);
    }
    .status-badge.warn {
      background: var(--warning-bg);
      border-color: rgba(251,191,36,0.2);
      color: var(--warning);
    }
    .status-badge.danger {
      background: var(--danger-bg);
      border-color: rgba(248,113,113,0.2);
      color: var(--danger);
    }
    .status-badge .dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: currentColor;
    }
    /* Cards */
    .card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 24px;
      transition: border-color var(--transition);
    }
    .card:hover {
      border-color: rgba(255,255,255,0.1);
    }
    .card-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 20px;
      flex-wrap: wrap;
    }
    .card-header h2 {
      font-size: 1rem;
      font-weight: 700;
      color: var(--text);
      letter-spacing: -0.01em;
    }
    .card-header p {
      color: var(--text-tertiary);
      font-size: 0.85rem;
      margin-top: 4px;
      line-height: 1.6;
    }
    .card-header-right {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
    }
    /* Stats grid */
    .stats-grid {
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 12px;
      margin-bottom: 16px;
    }
    .stat-card {
      background: rgba(255,255,255,0.03);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      padding: 16px 18px;
      display: grid;
      gap: 6px;
    }
    .stat-card .stat-label {
      font-size: 0.72rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--text-tertiary);
    }
    .stat-card .stat-value {
      font-size: 1.75rem;
      font-weight: 800;
      letter-spacing: -0.03em;
      color: var(--text);
      line-height: 1;
    }
    .stat-card .stat-value.accent { color: var(--accent); }
    .stat-card .stat-value.success { color: var(--success); }
    /* Meta rows */
    .meta-row {
      display: flex;
      align-items: center;
      gap: 12px;
      flex-wrap: wrap;
    }
    .meta-item {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 8px 14px;
      background: rgba(255,255,255,0.03);
      border: 1px solid var(--border);
      border-radius: 8px;
      font-size: 0.8rem;
    }
    .meta-item .meta-label {
      color: var(--text-tertiary);
      font-weight: 500;
    }
    .meta-item .meta-value {
      color: var(--text-secondary);
      font-family: var(--mono);
      font-size: 0.75rem;
    }
    /* Grid layout */
    .grid-2col {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 16px;
      margin-bottom: 16px;
    }
    .grid-full {
      grid-column: 1 / -1;
    }
    /* Fields */
    .field { display: grid; gap: 6px; }
    .field label {
      font-size: 0.75rem;
      font-weight: 600;
      color: var(--text-secondary);
      letter-spacing: 0.02em;
    }
    .field input, .field textarea, .field select {
      width: 100%;
      padding: 10px 14px;
      background: rgba(255,255,255,0.04);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      color: var(--text);
      font-family: var(--mono);
      font-size: 0.85rem;
      outline: none;
      transition: border-color var(--transition), box-shadow var(--transition);
    }
    .field input:focus, .field textarea:focus, .field select:focus {
      border-color: var(--border-focus);
      box-shadow: 0 0 0 3px var(--accent-glow);
    }
    .field input::placeholder, .field textarea::placeholder {
      color: var(--text-tertiary);
    }
    .field textarea {
      min-height: 90px;
      resize: vertical;
      line-height: 1.6;
    }
    .field select {
      cursor: pointer;
      appearance: none;
      background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' fill='%23a1a1aa' viewBox='0 0 16 16'%3E%3Cpath d='M8 11L3 6h10z'/%3E%3C/svg%3E");
      background-repeat: no-repeat;
      background-position: right 12px center;
      padding-right: 32px;
    }
    .field input[type="checkbox"] {
      width: 18px;
      height: 18px;
      accent-color: var(--accent);
      cursor: pointer;
    }
    /* Buttons */
    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      padding: 9px 18px;
      border: none;
      border-radius: var(--radius);
      font-family: var(--font);
      font-size: 0.85rem;
      font-weight: 600;
      cursor: pointer;
      transition: all var(--transition);
      white-space: nowrap;
    }
    .btn:active { transform: scale(0.97); }
    .btn-primary {
      background: linear-gradient(135deg, var(--accent), #7c3aed);
      color: white;
      box-shadow: 0 4px 16px var(--accent-glow);
    }
    .btn-primary:hover {
      box-shadow: 0 6px 24px rgba(139,92,246,0.35);
      transform: translateY(-1px);
    }
    .btn-secondary {
      background: var(--surface);
      color: var(--text);
      border: 1px solid var(--border);
    }
    .btn-secondary:hover {
      background: var(--surface-hover);
      border-color: rgba(255,255,255,0.15);
    }
    .btn-danger {
      background: var(--danger-bg);
      color: var(--danger);
      border: 1px solid rgba(248,113,113,0.15);
    }
    .btn-danger:hover {
      background: rgba(248,113,113,0.15);
    }
    .btn-ghost {
      background: transparent;
      color: var(--text-secondary);
      padding: 6px 12px;
    }
    .btn-ghost:hover {
      background: var(--surface);
      color: var(--text);
    }
    .btn-sm { padding: 6px 12px; font-size: 0.8rem; }
    .btn-xs { padding: 4px 10px; font-size: 0.75rem; }
    /* Row items */
    .row-list { display: grid; gap: 8px; }
    .row-item {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 12px 16px;
      background: rgba(255,255,255,0.02);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      transition: border-color var(--transition);
    }
    .row-item:hover {
      border-color: rgba(255,255,255,0.1);
    }
    .row-item input[type="text"],
    .row-item input[type="password"] {
      flex: 1;
      background: transparent;
      border: none;
      color: var(--text);
      font-family: var(--mono);
      font-size: 0.85rem;
      outline: none;
      padding: 4px 0;
    }
    .row-item input::placeholder { color: var(--text-tertiary); }
    .row-item .btn { flex-shrink: 0; }
    /* Account card */
    .account-card {
      background: rgba(255,255,255,0.02);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 20px;
      display: grid;
      gap: 16px;
    }
    .account-card:hover {
      border-color: rgba(255,255,255,0.1);
    }
    .account-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      flex-wrap: wrap;
    }
    .account-title {
      font-weight: 700;
      font-size: 0.95rem;
      color: var(--text);
    }
    .account-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
      gap: 12px;
    }
    .account-grid .field label {
      font-size: 0.7rem;
    }
    .account-grid .field input,
    .account-grid .field select {
      padding: 8px 12px;
      font-size: 0.8rem;
    }
    /* Model checkbox rows inside account card */
    .models-wrap {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      margin-top: 6px;
    }
    .model-chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 10px;
      background: rgba(255,255,255,0.04);
      border: 1px solid var(--border);
      border-radius: 6px;
      font-size: 0.75rem;
      cursor: pointer;
      transition: all var(--transition);
      user-select: none;
    }
    .model-chip:hover {
      border-color: rgba(139,92,246,0.3);
    }
    .model-chip.active {
      background: rgba(139,92,246,0.12);
      border-color: var(--accent);
      color: var(--accent-hover);
    }
    .model-chip input { display: none; }
    /* Toolbar */
    .toolbar {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
    }
    /* Note */
    .note {
      display: flex;
      align-items: flex-start;
      gap: 10px;
      margin-top: 16px;
      padding: 12px 16px;
      background: var(--warning-bg);
      border: 1px solid rgba(251,191,36,0.15);
      border-radius: var(--radius);
      color: var(--warning);
      font-size: 0.8rem;
      line-height: 1.6;
    }
    .note svg { flex-shrink: 0; margin-top: 1px; }
    /* Empty state */
    .empty {
      padding: 24px;
      border: 1px dashed rgba(255,255,255,0.1);
      border-radius: var(--radius);
      color: var(--text-tertiary);
      text-align: center;
      font-size: 0.85rem;
    }
    /* Section divider */
    .section-divider {
      margin: 16px 0;
      height: 1px;
      background: var(--border);
    }
    /* Hidden */
    .hidden { display: none !important; }
    /* Animations */
    @keyframes fadeIn {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }
    .card, .stat-card, .row-item, .account-card {
      animation: fadeIn 0.3s ease both;
    }
    .stat-card:nth-child(1) { animation-delay: 0.02s; }
    .stat-card:nth-child(2) { animation-delay: 0.04s; }
    .stat-card:nth-child(3) { animation-delay: 0.06s; }
    .stat-card:nth-child(4) { animation-delay: 0.08s; }
    /* Responsive */
    @media (max-width: 1024px) {
      .hero-grid { grid-template-columns: 1fr; }
      .stats-grid { grid-template-columns: repeat(2, 1fr); }
      .grid-2col { grid-template-columns: 1fr; }
    }
    @media (max-width: 640px) {
      .app { padding: 16px 12px 32px; }
      .hero { padding: 24px; }
      .hero h1 { font-size: 1.6rem; }
      .stats-grid { grid-template-columns: 1fr; }
      .card { padding: 18px; }
      .account-grid { grid-template-columns: 1fr; }
    }
    /* Toast notification */
    .toast-container {
      position: fixed;
      top: 20px;
      right: 20px;
      z-index: 9999;
      display: grid;
      gap: 10px;
      pointer-events: none;
    }
    .toast {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 12px 20px;
      background: rgba(28,28,46,0.95);
      backdrop-filter: blur(16px);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      box-shadow: var(--shadow-lg);
      font-size: 0.85rem;
      color: var(--text);
      pointer-events: auto;
      animation: slideIn 0.25s ease both;
    }
    .toast.ok { border-left: 3px solid var(--success); }
    .toast.warn { border-left: 3px solid var(--warning); }
    .toast.danger { border-left: 3px solid var(--danger); }
    @keyframes slideIn {
      from { opacity: 0; transform: translateX(20px); }
      to { opacity: 1; transform: translateX(0); }
    }
  </style>
</head>
<body>
  <div class="app">
    <!-- Hero / Login -->
    <div class="hero">
      <div class="hero-grid">
        <div>
          <h1>chat2api</h1>
          <p class="subtitle">管理面板 · 配置上游 ChatGPT 账号池、API Keys、直传前缀和全局代理。所有改动即时生效。</p>
        </div>
        <div class="login-card">
          <h2>后台登录</h2>
          <p>使用环境变量 ADMIN_USERNAME / ADMIN_PASSWORD 登录</p>
          <div class="field">
            <label for="username">Username</label>
            <input id="username" autocomplete="username" placeholder="输入用户名">
          </div>
          <div class="field">
            <label for="password">Password</label>
            <input id="password" type="password" autocomplete="current-password" placeholder="输入密码">
          </div>
          <div class="toolbar">
            <button class="btn btn-primary" id="loginBtn">登录</button>
            <button class="btn btn-secondary" id="logoutBtn">退出</button>
          </div>
          <div style="margin-top:12px">
            <span class="status-badge" id="heroStatus"><span class="dot"></span>未登录</span>
          </div>
        </div>
      </div>
    </div>
    <!-- Admin App -->
    <main id="adminApp" class="hidden">
      <!-- Status & Config -->
      <div class="card" style="margin-bottom:16px">
        <div class="card-header">
          <div>
            <h2>运行状态</h2>
            <p>配置改动会写回当前 YAML 文件，并即时刷新运行时内存。Render 未挂持久磁盘时，重建后文件更改可能丢失。</p>
          </div>
          <div class="card-header-right">
            <button class="btn btn-secondary btn-sm" id="exportBtn">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
              导出
            </button>
            <label class="btn btn-secondary btn-sm" for="importFile">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
              导入
            </label>
            <input id="importFile" type="file" accept=".yaml,.yml" class="hidden">
            <button class="btn btn-primary btn-sm" id="saveBtn">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg>
              保存全部
            </button>
          </div>
        </div>
        <div class="stats-grid">
          <div class="stat-card"><div class="stat-label">本地 API Keys</div><div class="stat-value accent" id="statAuth">0</div></div>
          <div class="stat-card"><div class="stat-label">直传前缀</div><div class="stat-value accent" id="statPrefixes">0</div></div>
          <div class="stat-card"><div class="stat-label">上游账号</div><div class="stat-value accent" id="statAccounts">0</div></div>
          <div class="stat-card"><div class="stat-label">可写配置</div><div class="stat-value success" id="statWritable">否</div></div>
        </div>
        <div class="meta-row">
          <div class="meta-item"><span class="meta-label">Config Path</span><span class="meta-value" id="configPath">未加载</span></div>
          <div class="meta-item"><span class="meta-label">Runtime Bind</span><span class="meta-value" id="runtimeBind">未加载</span></div>
          <span class="status-badge" id="saveStatus">等待加载</span>
        </div>
        <div class="note">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
          <span>高危提示：这里会显示和保存真实 token。只在可信网络和 HTTPS 下使用，后台密码必须足够强。</span>
        </div>
      </div>
      <!-- Models Summary -->
      <div class="card" style="margin-bottom:16px">
        <div class="card-header">
          <div>
            <h2>模型汇总</h2>
            <p>每个上游账号单独探测模型后，可勾选要启用的模型。/v1/models 仅返回所有账号已勾选模型的去重汇总。</p>
          </div>
          <div class="card-header-right">
            <span class="status-badge" id="modelsStatus"><span class="dot"></span>尚未加载</span>
          </div>
        </div>
        <div class="row-list" id="modelsList"></div>
      </div>
      <!-- Two-column section -->
      <div class="grid-2col">
        <div class="card">
          <div class="card-header">
            <div>
              <h2>本地 API Keys</h2>
              <p>用于调用 /v1/*。建议至少保留一个强随机 Key。</p>
            </div>
            <div class="card-header-right">
              <button class="btn btn-secondary btn-sm" id="generateAuthBtn">随机生成</button>
              <button class="btn btn-secondary btn-sm" id="addAuthBtn">+ 新增</button>
            </div>
          </div>
          <div class="row-list" id="authTokenList"></div>
        </div>
        <div class="card">
          <div class="card-header">
            <div>
              <h2>直传前缀</h2>
              <p>配置后可用 Authorization 直传上游 Access Token。</p>
            </div>
            <div class="card-header-right">
              <button class="btn btn-secondary btn-sm" id="addPrefixBtn">+ 新增</button>
            </div>
          </div>
          <div class="row-list" id="prefixList"></div>
        </div>
      </div>
      <!-- Global Settings -->
      <div class="card" style="margin-bottom:16px">
        <div class="card-header">
          <div>
            <h2>全局设置</h2>
            <p>业务连接配置。轮询模式按优先级从小到大依次尝试账号。</p>
          </div>
        </div>
        <div class="account-grid">
          <div class="field"><label for="globalProxy">Proxy</label><input id="globalProxy" placeholder="http://127.0.0.1:7890"></div>
          <div class="field"><label for="globalBaseUrl">ChatGPT Base URL</label><input id="globalBaseUrl" placeholder="https://chatgpt.com"></div>
          <div class="field"><label for="accountRoutingMode">上游账号模式</label><select id="accountRoutingMode"><option value="round_robin">轮询</option><option value="single">单一账号</option></select></div>
          <div class="field"><label for="selectedAccount">单一账号选择</label><select id="selectedAccount"><option value="">请选择账号</option></select></div>
        </div>
      </div>
      <!-- Account Pool -->
      <div class="card">
        <div class="card-header">
          <div>
            <h2>上游账号池</h2>
            <p>支持逐条增删改。access_token 为必填项。获取方式：登录 chatgpt.com → 打开 /api/auth/session → 复制 accessToken。</p>
          </div>
          <div class="card-header-right">
            <button class="btn btn-secondary btn-sm" id="importBtn">批量导入</button>
            <button class="btn btn-secondary btn-sm" id="addAccountBtn">+ 新增账号</button>
          </div>
        </div>
        <div class="field" style="margin-bottom:16px">
          <label for="bulkTokens">批量导入 Token</label>
          <textarea id="bulkTokens" placeholder="每行一个 token，也支持 token,proxy,email,type 四列逗号格式"></textarea>
        </div>
        <div class="account-list" id="accountList"></div>
      </div>
    </main>
  </div>
  <!-- Toast container -->
  <div class="toast-container" id="toastContainer"></div>
  <script>
    const $ = id => document.getElementById(id);
    const toastContainer = $('toastContainer');
    function toast(msg, kind) {
      const t = document.createElement('div');
      t.className = 'toast' + (kind ? ' ' + kind : '');
      t.textContent = msg;
      toastContainer.appendChild(t);
      setTimeout(() => { t.style.opacity = '0'; t.style.transition = 'opacity 0.3s'; setTimeout(() => t.remove(), 300); }, 3000);
    }
    const adminApp = $('adminApp'), heroStatus = $('heroStatus'), saveStatus = $('saveStatus');
    const authTokenList = $('authTokenList'), prefixList = $('prefixList'), accountList = $('accountList');
    const bulkTokens = $('bulkTokens'), globalProxy = $('globalProxy'), globalBaseUrl = $('globalBaseUrl');
    const accountRoutingMode = $('accountRoutingMode'), selectedAccount = $('selectedAccount');
    const modelsList = $('modelsList'), modelsStatus = $('modelsStatus');
    const statAuth = $('statAuth'), statPrefixes = $('statPrefixes'), statAccounts = $('statAccounts');
    const statWritable = $('statWritable'), configPath = $('configPath'), runtimeBind = $('runtimeBind');
    function setHero(text, kind) {
      heroStatus.innerHTML = '<span class="dot"></span>' + text;
      heroStatus.className = 'status-badge';
      if (kind === 'ok') heroStatus.classList.add('ok');
      if (kind === 'warn') heroStatus.classList.add('warn');
      if (kind === 'danger') heroStatus.classList.add('danger');
    }
    function setSave(text, kind) {
      saveStatus.innerHTML = '<span class="dot"></span>' + text;
      saveStatus.className = 'status-badge';
      if (kind === 'ok') saveStatus.classList.add('ok');
      if (kind === 'warn') saveStatus.classList.add('warn');
    }
    function setModels(text, kind) {
      modelsStatus.innerHTML = '<span class="dot"></span>' + text;
      modelsStatus.className = 'status-badge';
      if (kind === 'ok') modelsStatus.classList.add('ok');
      if (kind === 'warn') modelsStatus.classList.add('warn');
    }
    async function parseResponse(r) {
      const d = await r.json().catch(() => ({}));
      if (!r.ok) {
        const detail = d && d.detail;
        throw new Error(detail && detail.msg ? String(detail.msg) : '请求失败: ' + r.status);
      }
      if (d && d.code && d.code !== 0) throw new Error(d.detail || d.message || '接口返回失败');
      return d.data;
    }
    async function login() {
      setHero('正在登录...');
      try {
        const r = await fetch('/admin/api/login', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify({ username: $('username').value.trim(), password: $('password').value })
        });
        await parseResponse(r);
        $('password').value = '';
        setHero('已登录', 'ok');
        toast('登录成功', 'ok');
        adminApp.classList.remove('hidden');
        await loadConfig();
        await loadModels();
      } catch (e) {
        setHero(e.message, 'warn');
        toast(e.message, 'danger');
      }
    }
    async function logout() {
      await fetch('/admin/api/logout', { method: 'POST', credentials: 'same-origin' }).catch(() => {});
      adminApp.classList.add('hidden');
      clearNode(modelsList);
      modelsList.appendChild(empty('登录后可查看模型探测结果。'));
      setModels('已退出');
      setHero('已退出');
      toast('已退出登录', 'warn');
    }
    function clearNode(n) { while (n.firstChild) n.removeChild(n.firstChild); }
    function empty(t) {
      const d = document.createElement('div');
      d.className = 'empty'; d.textContent = t; return d;
    }
    function normalize() {
      [authTokenList, prefixList, accountList].forEach(n =>
        Array.from(n.querySelectorAll('.empty')).forEach(i => i.remove())
      );
    }
    function ensureEmpty() {
      if (!authTokenList.children.length) authTokenList.appendChild(empty('当前没有本地 API Key。'));
      if (!prefixList.children.length) prefixList.appendChild(empty('当前没有 Access Token 前缀。'));
      if (!accountList.children.length) accountList.appendChild(empty('当前没有上游账号。'));
    }
    function simpleRow(v, p) {
      const w = document.createElement('div');
      w.className = 'row-item';
      const i = document.createElement('input');
      i.value = v || ''; i.placeholder = p;
      i.type = 'text'; i.spellcheck = false;
      const b = document.createElement('button');
      b.className = 'btn btn-danger btn-xs';
      b.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>';
      b.onclick = () => { w.remove(); refresh(); ensureEmpty(); };
      w.append(i, b);
      return w;
    }
    function addAuth(v) { normalize(); authTokenList.appendChild(simpleRow(v, 'sk-your-local-key')); refresh(); }
    function addPrefix(v) { normalize(); prefixList.appendChild(simpleRow(v, 'your-private-prefix-')); refresh(); }
    function field(label, name, value, placeholder, type) {
      const f = document.createElement('div');
      f.className = 'field';
      const l = document.createElement('label');
      l.textContent = label;
      const i = document.createElement(type === 'textarea' ? 'textarea' : 'input');
      if (type && type !== 'textarea') i.type = type;
      i.dataset.name = name;
      i.value = value == null ? '' : String(value);
      i.placeholder = placeholder || '';
      f.append(l, i);
      return f;
    }
    function checkboxField(label, name, checked) {
      const f = document.createElement('div');
      f.className = 'field';
      const l = document.createElement('label');
      l.textContent = label;
      const i = document.createElement('input');
      i.type = 'checkbox'; i.dataset.name = name;
      i.checked = !!checked;
      f.append(l, i);
      return f;
    }
    function hasAccountContent(account) {
      return !!(String(account.access_token || '').trim() || String(account.proxy || '').trim() ||
        String(account.email || '').trim() || String(account.type || '').trim() ||
        String(account.account_id || '').trim() || String(account.id_token || '').trim() ||
        String(account.refresh_token || '').trim() || String(account.last_refresh || '').trim() ||
        String(account.expired || '').trim());
    }
    function updateSelectedAccountOptions(preferred) {
      const current = preferred != null ? String(preferred) : selectedAccount.value;
      clearNode(selectedAccount);
      const emptyOption = document.createElement('option');
      emptyOption.value = ''; emptyOption.textContent = '请选择账号';
      selectedAccount.appendChild(emptyOption);
      accounts().forEach((account, index) => {
        const opt = document.createElement('option');
        opt.value = account.id || account.email || account.account_id || account.access_token;
        opt.textContent = (account.email || account.account_id || ('账号' + String(index + 1))) +
          ' | ' + (account.enabled ? '启用' : '停用') + ' | 优先级 ' + String(account.priority || 0);
        selectedAccount.appendChild(opt);
      });
      selectedAccount.value = current && Array.from(selectedAccount.options).some(o => o.value === current) ? current : '';
      selectedAccount.disabled = accountRoutingMode.value !== 'single';
    }
    function renderAccountModels(container, account) {
      container.innerHTML = '';
      const available = (account.available_models || []);
      if (!available.length) {
        container.appendChild(empty('先点击"探测模型"获取该账号可用模型。'));
        return;
      }
      const selected = new Set(account.selected_models || []);
      available.forEach(model => {
        const chip = document.createElement('label');
        chip.className = 'model-chip' + (selected.has(model) ? ' active' : '');
        const box = document.createElement('input');
        box.type = 'checkbox'; box.dataset.modelId = model;
        box.checked = selected.has(model);
        box.onchange = () => {
          chip.classList.toggle('active', box.checked);
          const card = chip.closest('.account-card');
          const current = readAccountCard(card);
          const wrap = card.querySelector('[data-role="models-wrap"]');
          current.selected_models = Array.from(wrap.querySelectorAll('input[data-model-id]:checked')).map(i => i.dataset.modelId);
          writeAccountModels(card, current);
          refresh();
          updateSelectedAccountOptions();
          refreshSummaryModels();
        };
        const span = document.createElement('span');
        span.textContent = model;
        chip.append(box, span);
        container.appendChild(chip);
      });
    }
    async function probeAccountModels(card) {
      const account = readAccountCard(card);
      const status = card.querySelector('[data-role="probe-status"]');
      const modelsWrap = card.querySelector('[data-role="models-wrap"]');
      status.textContent = '探测中...';
      status.className = 'status-badge';
      try {
        const r = await fetch('/admin/api/models/probe', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify({ account_id: account.id || account.email || account.account_id, access_token: account.access_token, proxy: account.proxy })
        });
        const data = await parseResponse(r);
        account.available_models = data.available_models || [];
        account.selected_models = data.available_models || [];
        writeAccountModels(card, account);
        renderAccountModels(modelsWrap, account);
        status.innerHTML = '<span class="dot"></span>已探测 ' + String(data.available || 0) + ' / ' + String(data.total || 0) + (data.source === 'chatgpt_web_models' ? ' | 网页模型列表' : ' | 回退候选探测');
        status.className = 'status-badge ok';
        toast('模型探测完成: ' + String(data.available || 0) + ' 个可用', 'ok');
        refreshSummaryModels();
      } catch (e) {
        status.textContent = e.message;
        status.className = 'status-badge warn';
        toast(e.message, 'danger');
      }
    }
    function writeAccountModels(card, account) {
      card.dataset.availableModels = JSON.stringify(account.available_models || []);
      card.dataset.selectedModels = JSON.stringify(account.selected_models || []);
    }
    function readAccountModels(card) {
      return { available_models: JSON.parse(card.dataset.availableModels || '[]'), selected_models: JSON.parse(card.dataset.selectedModels || '[]') };
    }
    function readAccountCard(card) {
      const d = {};
      card.querySelectorAll('[data-name]').forEach(i => {
        if (i.type === 'checkbox') { d[i.dataset.name] = i.checked; return; }
        d[i.dataset.name] = i.value.trim();
      });
      d.priority = Number.parseInt(d.priority || '0', 10);
      if (Number.isNaN(d.priority)) d.priority = 0;
      const models = readAccountModels(card);
      d.available_models = models.available_models;
      d.selected_models = models.selected_models;
      return d;
    }
    function accountCard(a) {
      a = a || {};
      const c = document.createElement('div');
      c.className = 'account-card';
      writeAccountModels(c, a);
      const h = document.createElement('div');
      h.className = 'account-head';
      const t = document.createElement('div');
      t.className = 'account-title';
      t.textContent = a.email || a.account_id || a.id || '新账号';
      const tools = document.createElement('div');
      tools.className = 'toolbar';
      const probe = document.createElement('button');
      probe.className = 'btn btn-secondary btn-xs';
      probe.innerHTML = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg> 探测模型';
      probe.onclick = () => probeAccountModels(c);
      const rm = document.createElement('button');
      rm.className = 'btn btn-danger btn-xs';
      rm.innerHTML = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>';
      rm.onclick = () => { c.remove(); refresh(); ensureEmpty(); updateSelectedAccountOptions(); refreshSummaryModels(); };
      tools.append(probe, rm);
      h.append(t, tools);
      const g = document.createElement('div');
      g.className = 'account-grid';
      g.appendChild(field('Selector ID', 'id', a.id, 'auto_or_custom_id'));
      g.appendChild(checkboxField('Enabled', 'enabled', a.enabled !== false));
      g.appendChild(field('Priority', 'priority', a.priority || 0, '0', 'number'));
      [['Access Token', 'access_token', 'real_access_token'],
       ['Proxy', 'proxy', 'http://127.0.0.1:7890'],
       ['Email', 'email', 'user@example.com'],
       ['Type', 'type', 'codex'],
       ['Account ID', 'account_id', 'optional_account_id'],
       ['ID Token', 'id_token', 'optional_id_token'],
       ['Refresh Token', 'refresh_token', 'optional_refresh_token'],
       ['Expired', 'expired', ''],
       ['Last Refresh', 'last_refresh', '']].forEach(x => g.appendChild(field(x[0], x[1], a[x[1]], x[2])));
      const modelBox = document.createElement('div');
      modelBox.className = 'field';
      const modelLabel = document.createElement('label');
      modelLabel.textContent = '账号模型';
      const modelStatus = document.createElement('div');
      modelStatus.dataset.role = 'probe-status';
      modelStatus.className = 'status-badge';
      modelStatus.textContent = (a.available_models && a.available_models.length) ? '已加载模型' : '尚未探测';
      const modelsWrap = document.createElement('div');
      modelsWrap.dataset.role = 'models-wrap';
      modelsWrap.className = 'models-wrap';
      renderAccountModels(modelsWrap, a);
      modelBox.append(modelLabel, modelStatus, modelsWrap);
      g.appendChild(modelBox);
      g.oninput = () => {
        const email = g.querySelector('input[data-name="email"]').value.trim();
        const token = g.querySelector('input[data-name="access_token"]').value.trim();
        const idv = g.querySelector('input[data-name="id"]').value.trim();
        t.textContent = email || idv || token || '新账号';
        refresh();
        updateSelectedAccountOptions();
        refreshSummaryModels();
      };
      g.onchange = (e) => {
        if (e.target && e.target.dataset && e.target.dataset.modelId) {
          const current = readAccountCard(c);
          current.selected_models = Array.from(modelsWrap.querySelectorAll('input[data-model-id]:checked')).map(i => i.dataset.modelId);
          writeAccountModels(c, current);
        }
        refresh();
        updateSelectedAccountOptions();
        refreshSummaryModels();
      };
      c.append(h, g);
      return c;
    }
    function addAccount(a) { normalize(); accountList.appendChild(accountCard(a)); refresh(); updateSelectedAccountOptions(); refreshSummaryModels(); }
    function listValues(n) {
      return Array.from(n.children).filter(i => !i.classList.contains('empty'))
        .map(i => i.querySelector('input')).filter(Boolean).map(i => i.value.trim()).filter(Boolean);
    }
    function accounts() {
      return Array.from(accountList.children).filter(i => !i.classList.contains('empty')).map(readAccountCard).filter(hasAccountContent);
    }
    function refresh() {
      statAuth.textContent = listValues(authTokenList).length;
      statPrefixes.textContent = listValues(prefixList).length;
      statAccounts.textContent = accounts().length;
    }
    function refreshSummaryModels() {
      const seen = new Set();
      const summary = [];
      accounts().filter(a => a.enabled !== false).forEach(account => {
        (account.selected_models || []).forEach(model => {
          if (!seen.has(model)) { seen.add(model); summary.push(model); }
        });
      });
      renderModels({ summary_models: summary });
    }
    function render(p) {
      clearNode(authTokenList); clearNode(prefixList); clearNode(accountList);
      (p.auth_tokens || []).forEach(addAuth);
      (p.access_token_prefixes || []).forEach(addPrefix);
      (p.chatgpt_accounts || []).forEach(addAccount);
      globalProxy.value = p.proxy || '';
      globalBaseUrl.value = p.chatgpt_base_url || '';
      accountRoutingMode.value = p.account_routing_mode || 'round_robin';
      configPath.textContent = p.config_path || '当前运行态未绑定可写配置文件';
      runtimeBind.textContent = (p.runtime_bind || '-') + ':' + String(p.runtime_port || '-');
      statWritable.textContent = p.writable ? '是' : '否';
      statWritable.className = 'stat-value' + (p.writable ? ' success' : '');
      refresh(); ensureEmpty();
      updateSelectedAccountOptions(p.selected_account || '');
      renderModels({ summary_models: p.summary_models || [] });
      setSave(p.writable ? '配置已加载，可保存' : '当前运行态只读，不可保存', p.writable ? 'ok' : 'warn');
    }
    function renderModels(payload) {
      clearNode(modelsList);
      const models = (payload.summary_models || []);
      if (!models.length) {
        modelsList.appendChild(empty('当前没有已启用的汇总模型。'));
        setModels('尚未选择模型', 'warn');
        return;
      }
      models.forEach(model => {
        const row = document.createElement('div');
        row.className = 'row-item';
        const left = document.createElement('div');
        left.innerHTML = '<div style="font-weight:700;font-size:0.9rem">' + model + '</div><div style="font-size:0.75rem;color:var(--text-tertiary);margin-top:2px">/v1/models 将返回该模型</div>';
        const badge = document.createElement('span');
        badge.className = 'status-badge ok';
        badge.innerHTML = '<span class="dot"></span>已汇总';
        row.append(left, badge);
        modelsList.appendChild(row);
      });
      setModels('当前汇总 ' + String(models.length) + ' 个模型', 'ok');
    }
    async function loadConfig() {
      try {
        const r = await fetch('/admin/api/config', { credentials: 'same-origin' });
        render(await parseResponse(r));
        adminApp.classList.remove('hidden');
        setHero('已登录', 'ok');
      } catch (e) {
        if (String(e.message).includes('401')) {
          adminApp.classList.add('hidden');
          setHero('未登录');
          setSave('登录后可加载配置', 'warn');
          return;
        }
        setSave(e.message, 'warn');
        toast(e.message, 'danger');
      }
    }
    async function loadModels() {
      try {
        const r = await fetch('/admin/api/models', { credentials: 'same-origin' });
        renderModels(await parseResponse(r));
      } catch (e) {
        if (String(e.message).includes('401')) { setModels('登录后可查看模型汇总', 'warn'); return; }
        setModels(e.message, 'warn');
      }
    }
    async function saveConfig() {
      const payload = {
        proxy: globalProxy.value.trim(), chatgpt_base_url: globalBaseUrl.value.trim(),
        account_routing_mode: accountRoutingMode.value, selected_account: selectedAccount.value,
        auth_tokens: listValues(authTokenList), access_token_prefixes: listValues(prefixList),
        chatgpt_accounts: accounts()
      };
      setSave('正在保存配置...');
      try {
        const r = await fetch('/admin/api/config', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin', body: JSON.stringify(payload) });
        await parseResponse(r);
        setSave('配置已保存并刷新运行时内存', 'ok');
        toast('配置已保存', 'ok');
        await loadConfig();
      } catch (e) { setSave(e.message, 'warn'); toast(e.message, 'danger'); }
    }
    async function generateAuthToken() {
      setSave('正在生成随机 Key...');
      try {
        const r = await fetch('/admin/api/auth-token/generate', { method: 'POST', credentials: 'same-origin' });
        const data = await parseResponse(r);
        if (!data || !data.token) throw new Error('随机生成失败');
        addAuth(data.token);
        setSave('已生成随机 Key，记得点击保存全部配置', 'ok');
        toast('已生成随机 Key', 'ok');
      } catch (e) { setSave(e.message, 'warn'); toast(e.message, 'danger'); }
    }
    async function exportConfig() {
      setSave('正在导出配置...');
      try {
        const r = await fetch('/admin/api/config/export', { credentials: 'same-origin' });
        if (!r.ok) throw new Error('导出失败: ' + r.status);
        const blob = await r.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url; a.download = 'chat2api-config.yaml';
        document.body.appendChild(a); a.click();
        a.remove(); URL.revokeObjectURL(url);
        setSave('配置已导出', 'ok');
        toast('配置已导出', 'ok');
      } catch (e) { setSave(e.message, 'warn'); toast(e.message, 'danger'); }
    }
    async function importConfig(file) {
      if (!file) return;
      const form = new FormData();
      form.append('file', file);
      setSave('正在导入配置...');
      try {
        const r = await fetch('/admin/api/config/import', { method: 'POST', credentials: 'same-origin', body: form });
        await parseResponse(r);
        setSave('配置已导入并刷新运行时内存', 'ok');
        toast('配置已导入', 'ok');
        await loadConfig();
        await loadModels();
      } catch (e) { setSave(e.message, 'warn'); toast(e.message, 'danger'); }
    }
    function importBulk() {
      const raw = bulkTokens.value.trim();
      if (!raw) { toast('批量导入内容为空', 'warn'); return; }
      raw.split(/\n+/).forEach(line => {
        const parts = line.trim().split(',').map(x => x.trim());
        if (parts[0]) addAccount({ access_token: parts[0], proxy: parts[1] || '', email: parts[2] || '', type: parts[3] || '', enabled: true, priority: 0 });
      });
      bulkTokens.value = '';
      toast('批量 Token 已加入待保存列表', 'ok');
    }
    accountRoutingMode.onchange = () => updateSelectedAccountOptions();
    $('loginBtn').onclick = login;
    $('logoutBtn').onclick = logout;
    $('saveBtn').onclick = saveConfig;
    $('exportBtn').onclick = exportConfig;
    $('importFile').onchange = (e) => { const file = e.target.files && e.target.files[0]; importConfig(file).finally(() => { e.target.value = ''; }); };
    $('generateAuthBtn').onclick = generateAuthToken;
    $('addAuthBtn').onclick = () => addAuth('');
    $('addPrefixBtn').onclick = () => addPrefix('');
    $('addAccountBtn').onclick = () => addAccount({ enabled: true, priority: 0, available_models: [], selected_models: [] });
    $('importBtn').onclick = importBulk;
    $('password').addEventListener('keydown', e => { if (e.key === 'Enter') login(); });
    loadConfig(); loadModels(); ensureEmpty(); updateSelectedAccountOptions();
  </script>
</body>
</html>`
