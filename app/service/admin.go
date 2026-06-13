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
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg: #fafbfc;
      --bg-card: #ffffff;
      --bg-sidebar: #f5f6f8;
      --bg-input: #f5f6f8;
      --bg-hover: #edeeef;
      --bg-active: #e5e6e8;
      --border: #e6e8ea;
      --border-light: #f0f1f2;
      --border-focus: #3b82f6;
      --text-primary: #0d0d0d;
      --text-secondary: #6b7280;
      --text-tertiary: #9ca3af;
      --text-placeholder: #bfc4cb;
      --accent: #3b82f6;
      --accent-hover: #2563eb;
      --accent-subtle: rgba(59,130,246,0.08);
      --success: #059669;
      --success-bg: rgba(5,150,105,0.08);
      --warning: #b45309;
      --warning-bg: rgba(180,83,9,0.08);
      --danger: #dc2626;
      --danger-bg: rgba(220,38,38,0.08);
      --font: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
      --mono: 'SF Mono', 'JetBrains Mono', 'Menlo', Consolas, monospace;
      --radius: 8px;
      --radius-card: 12px;
      --shadow: 0 1px 2px rgba(0,0,0,0.04), 0 0 0 1px rgba(0,0,0,0.02);
      --shadow-elevated: 0 1px 3px rgba(0,0,0,0.06), 0 0 0 1px rgba(0,0,0,0.03);
      --shadow-modal: 0 4px 24px rgba(0,0,0,0.08), 0 0 0 1px rgba(0,0,0,0.04);
      --sidebar-width: 240px;
      --header-height: 56px;
    }
    /* Reset */
    *,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
    html{font-size:14px;-webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale}
    body{font-family:var(--font);color:var(--text-primary);background:var(--bg);line-height:1.55;min-height:100vh}
    /* Scrollbar */
    ::-webkit-scrollbar{width:4px;height:4px}
    ::-webkit-scrollbar-track{background:transparent}
    ::-webkit-scrollbar-thumb{background:var(--text-tertiary);border-radius:2px}
    ::-webkit-scrollbar-thumb:hover{background:var(--text-secondary)}
    /* Layout */
    .app{display:flex;min-height:100vh}
    /* Sidebar */
    .sidebar{width:var(--sidebar-width);background:var(--bg-sidebar);border-right:1px solid var(--border);display:flex;flex-direction:column;position:fixed;top:0;left:0;bottom:0;z-index:40;padding:0}
    .sidebar-brand{height:var(--header-height);display:flex;align-items:center;padding:0 20px;border-bottom:1px solid var(--border);gap:10px;flex-shrink:0}
    .sidebar-brand-icon{width:28px;height:28px;border-radius:7px;background:var(--accent);display:flex;align-items:center;justify-content:center;color:#fff;font-weight:800;font-size:13px}
    .sidebar-brand-text{font-weight:700;font-size:14px;letter-spacing:-0.02em;color:var(--text-primary)}
    .sidebar-nav{flex:1;overflow-y:auto;padding:12px 8px;display:flex;flex-direction:column;gap:2px}
    .sidebar-section{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.06em;color:var(--text-tertiary);padding:12px 12px 6px}
    .sidebar-item{display:flex;align-items:center;gap:10px;padding:8px 12px;border-radius:7px;font-size:13px;font-weight:500;color:var(--text-secondary);cursor:pointer;transition:all 0.12s ease;text-decoration:none;user-select:none}
    .sidebar-item:hover{background:var(--bg-hover);color:var(--text-primary)}
    .sidebar-item.active{background:var(--accent-subtle);color:var(--accent);font-weight:600}
    .sidebar-item svg{flex-shrink:0;opacity:0.7}
    .sidebar-item.active svg{opacity:1}
    .sidebar-footer{padding:12px;border-top:1px solid var(--border);flex-shrink:0}
    .sidebar-footer .sidebar-item{font-size:12px}
    /* Main */
    .main{flex:1;margin-left:var(--sidebar-width);min-height:100vh}
    /* Header */
    .header{height:var(--header-height);display:flex;align-items:center;justify-content:space-between;padding:0 32px;border-bottom:1px solid var(--border-light);background:var(--bg-card);position:sticky;top:0;z-index:30}
    .header-left{display:flex;align-items:center;gap:12px}
    .header-breadcrumb{font-size:13px;font-weight:500;color:var(--text-secondary)}
    .header-breadcrumb span{color:var(--text-tertiary)}
    .header-title{font-size:15px;font-weight:700;color:var(--text-primary);letter-spacing:-0.01em}
    .header-right{display:flex;align-items:center;gap:8px}
    /* Content */
    .content{padding:32px;max-width:1200px}
    /* Page Header */
    .page-header{margin-bottom:32px}
    .page-header h1{font-size:22px;font-weight:700;letter-spacing:-0.03em;color:var(--text-primary);margin-bottom:6px}
    .page-header p{font-size:14px;color:var(--text-secondary);line-height:1.6}
    /* Card system */
    .card-grid{display:grid;gap:16px}
    .card-grid.half{grid-template-columns:1fr 1fr}
    .card-grid.quarter{grid-template-columns:repeat(4,1fr)}
    .card{background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius-card);box-shadow:var(--shadow);transition:box-shadow 0.15s ease}
    .card:hover{box-shadow:var(--shadow-elevated)}
    .card-body{padding:20px}
    .card-body.compact{padding:16px}
    .card-header{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;margin-bottom:16px}
    .card-header h3{font-size:13px;font-weight:600;color:var(--text-primary);letter-spacing:-0.01em}
    .card-header p{font-size:12px;color:var(--text-tertiary);margin-top:3px;line-height:1.5}
    .card-actions{display:flex;align-items:center;gap:6px;flex-shrink:0}
    /* Stats card */
    .stat-card{background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius-card);padding:20px;box-shadow:var(--shadow)}
    .stat-card .stat-label{font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;color:var(--text-tertiary);margin-bottom:6px}
    .stat-card .stat-value{font-size:28px;font-weight:700;letter-spacing:-0.03em;color:var(--text-primary);line-height:1}
    .stat-card .stat-desc{font-size:12px;color:var(--text-tertiary);margin-top:4px}
    html{scroll-behavior:smooth}
    /* Login Card */
    .login-page{min-height:100vh;display:flex;align-items:center;justify-content:center;background:var(--bg);padding:24px}
    .login-box{width:380px;max-width:100%}
    .login-box .card-body{padding:32px}
    .login-brand{display:flex;align-items:center;gap:10px;margin-bottom:4px}
    .login-brand-icon{width:36px;height:36px;border-radius:9px;background:var(--accent);display:flex;align-items:center;justify-content:center;color:#fff;font-weight:800;font-size:15px}
    .login-title{font-size:18px;font-weight:700;letter-spacing:-0.03em;color:var(--text-primary);margin-bottom:4px}
    .login-subtitle{font-size:13px;color:var(--text-tertiary);margin-bottom:24px;line-height:1.5}
    .login-box .field{margin-bottom:14px}
    .login-box .field:last-of-type{margin-bottom:20px}
    .login-status{margin-top:12px}
    /* Fields */
    .field{display:grid;gap:5px}
    .field label{font-size:12px;font-weight:600;color:var(--text-secondary);letter-spacing:0.01em}
    .field input,.field textarea,.field select{width:100%;padding:9px 12px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius);color:var(--text-primary);font-family:var(--mono);font-size:13px;outline:none;transition:border-color 0.12s ease,box-shadow 0.12s ease}
    .field input:focus,.field textarea:focus,.field select:focus{border-color:var(--border-focus);box-shadow:0 0 0 3px rgba(59,130,246,0.15)}
    .field input::placeholder,.field textarea::placeholder{color:var(--text-placeholder)}
    .field textarea{min-height:80px;resize:vertical;line-height:1.5;font-family:var(--mono)}
    .field select{cursor:pointer;appearance:none;background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' fill='%239ca3af' viewBox='0 0 16 16'%3E%3Cpath d='M8 12L4 8h8z'/%3E%3C/svg%3E");background-repeat:no-repeat;background-position:right 10px center;padding-right:28px;font-family:var(--font)}
    .field input[type="checkbox"]{width:16px;height:16px;accent-color:var(--accent);cursor:pointer}
    .field-group{display:grid;grid-template-columns:1fr 1fr;gap:14px}
    /* Buttons */
    .btn{display:inline-flex;align-items:center;justify-content:center;gap:6px;padding:7px 14px;border:none;border-radius:var(--radius);font-family:var(--font);font-size:13px;font-weight:600;cursor:pointer;transition:all 0.12s ease;white-space:nowrap;line-height:1;text-decoration:none}
    .btn:active{transform:scale(0.97)}
    .btn-primary{background:var(--accent);color:#fff}
    .btn-primary:hover{background:var(--accent-hover)}
    .btn-secondary{background:var(--bg-input);color:var(--text-primary);border:1px solid var(--border)}
    .btn-secondary:hover{background:var(--bg-hover)}
    .btn-danger{background:var(--danger-bg);color:var(--danger);border:1px solid rgba(220,38,38,0.12)}
    .btn-danger:hover{background:rgba(220,38,38,0.12)}
    .btn-ghost{background:transparent;color:var(--text-secondary)}
    .btn-ghost:hover{background:var(--bg-hover);color:var(--text-primary)}
    .btn-sm{padding:5px 10px;font-size:12px}
    .btn-xs{padding:3px 8px;font-size:11px}
    .btn-icon{padding:6px;min-width:30px;height:30px}
    /* Badge */
    .badge{display:inline-flex;align-items:center;gap:5px;padding:3px 10px;border-radius:999px;font-size:11px;font-weight:600;background:var(--bg-input);color:var(--text-secondary);border:1px solid var(--border-light)}
    .badge .dot{width:5px;height:5px;border-radius:50%;background:currentColor}
    .badge.ok{background:var(--success-bg);color:var(--success);border-color:rgba(5,150,105,0.12)}
    .badge.warn{background:var(--warning-bg);color:var(--warning);border-color:rgba(180,83,9,0.12)}
    .badge.danger{background:var(--danger-bg);color:var(--danger);border-color:rgba(220,38,38,0.12)}
    /* Row list — 更精致的 key 输入行 */
    .row-list{display:grid;gap:6px}
    .row-item{display:flex;align-items:center;gap:8px;padding:0;background:transparent;border:none;border-radius:0;transition:none}
    .row-item:hover{border-color:transparent}
    .row-item .key-field{flex:1;display:flex;align-items:center;gap:8px;padding:8px 12px;background:var(--bg-input);border:1px solid var(--border-light);border-radius:var(--radius);transition:border-color 0.12s ease,box-shadow 0.12s ease}
    .row-item .key-field:focus-within{border-color:var(--border-focus);box-shadow:0 0 0 3px rgba(59,130,246,0.12)}
    .row-item .key-field input{flex:1;background:transparent;border:none;color:var(--text-primary);font-family:var(--mono);font-size:13px;outline:none;padding:0}
    .row-item .key-field input::placeholder{color:var(--text-placeholder)}
    .row-item .key-field .key-icon{flex-shrink:0;color:var(--text-tertiary);display:flex;align-items:center}
    /* Account card */
    .account-card{background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius-card);overflow:hidden;box-shadow:var(--shadow)}
    .account-card:hover{box-shadow:var(--shadow-elevated)}
    .account-head{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 20px;border-bottom:1px solid var(--border-light);background:var(--bg)}
    .account-title{font-size:13px;font-weight:600;color:var(--text-primary)}
    .account-body{padding:20px;display:grid;gap:14px}
    .account-fields{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:12px}
    .account-fields .field label{font-size:11px}
    .account-fields .field input,.account-fields .field select{padding:7px 10px;font-size:12px}
    /* Model chips */
    .models-wrap{display:flex;flex-wrap:wrap;gap:5px;margin-top:6px}
    .model-chip{display:inline-flex;align-items:center;gap:5px;padding:3px 9px;background:var(--bg);border:1px solid var(--border-light);border-radius:5px;font-size:12px;cursor:pointer;transition:all 0.12s ease;user-select:none;font-family:var(--mono)}
    .model-chip:hover{border-color:var(--border)}
    .model-chip.active{background:var(--accent-subtle);border-color:var(--accent);color:var(--accent)}
    .model-chip input{display:none}
    /* Empty */
    .empty{padding:20px;border:1px dashed var(--border);border-radius:var(--radius);color:var(--text-tertiary);text-align:center;font-size:13px;background:var(--bg)}
    /* Note / Alert */
    .alert{display:flex;align-items:flex-start;gap:10px;padding:12px 16px;background:var(--warning-bg);border:1px solid rgba(180,83,9,0.12);border-radius:var(--radius);color:var(--warning);font-size:13px;line-height:1.5;margin-top:16px}
    .alert svg{flex-shrink:0;margin-top:1px;opacity:0.8}
    /* Section */
    .section{margin-bottom:24px}
    .section:last-child{margin-bottom:0}
    /* Collapse */
    .collapse-toggle{display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;padding:4px 0;color:var(--text-secondary);font-size:12px;font-weight:600}
    .collapse-toggle:hover{color:var(--text-primary)}
    .collapse-toggle svg{transition:transform 0.15s ease}
    .collapse-toggle.collapsed svg{transform:rotate(-90deg)}
    .collapse-content{overflow:hidden;transition:max-height 0.2s ease,opacity 0.15s ease}
    .collapse-content.hidden{max-height:0;opacity:0;pointer-events:none}
    /* Divider */
    .divider{height:1px;background:var(--border-light);margin:16px 0}
    /* Hidden */
    .hidden{display:none!important}
    /* Toast */
    .toast-container{position:fixed;bottom:24px;right:24px;z-index:9999;display:grid;gap:8px;pointer-events:none}
    .toast{display:flex;align-items:center;gap:10px;padding:10px 16px;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);box-shadow:var(--shadow-modal);font-size:13px;color:var(--text-primary);pointer-events:auto;animation:fadeSlideUp 0.2s ease both}
    .toast.ok{border-left:3px solid var(--success)}
    .toast.warn{border-left:3px solid var(--warning)}
    .toast.danger{border-left:3px solid var(--danger)}
    @keyframes fadeSlideUp{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:translateY(0)}}
    /* Responsive */
    @media(max-width:1024px){
      .sidebar{display:none}
      .main{margin-left:0}
      .header,.content{padding-left:20px;padding-right:20px}
      .card-grid.half,.card-grid.quarter{grid-template-columns:1fr}
      .field-group{grid-template-columns:1fr}
    }
  </style>
</head>
<body>
<div class="app">
  <!-- Sidebar -->
  <aside class="sidebar">
    <div class="sidebar-brand"><div class="sidebar-brand-icon">C</div><span class="sidebar-brand-text">chat2api</span></div>
    <nav class="sidebar-nav">
      <div class="sidebar-section">Main</div>
      <a class="sidebar-item active" id="navDashboard">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
        Dashboard
      </a>
      <a class="sidebar-item" id="navAccounts">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
        上游账户
      </a>
      <a class="sidebar-item" id="navKeys">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>
        API Keys
      </a>
      <a class="sidebar-item" id="navSettings">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
        全局设置
      </a>
    </nav>
    <div class="sidebar-footer">
      <div class="sidebar-item" id="navLogout" style="cursor:pointer">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
        退出登录
      </div>
    </div>
  </aside>
  <!-- Main -->
  <div class="main">
    <!-- Header -->
    <header class="header">
      <div class="header-left">
        <div class="header-breadcrumb"><span>chat2api</span> / 管理面板</div>
      </div>
      <div class="header-right" id="loginInfo">
        <span class="badge" id="heroStatus"><span class="dot"></span>未登录</span>
      </div>
    </header>
    <!-- Content -->
    <div class="content" id="adminContent">
      <!-- Login screen -->
      <div id="loginScreen">
        <div class="login-page" style="padding-top:10vh">
          <div class="login-box">
            <div class="card">
              <div class="card-body">
                <div class="login-brand"><div class="login-brand-icon">C</div></div>
                <div class="login-title">管理面板登录</div>
                <div class="login-subtitle">使用 ADMIN_USERNAME / ADMIN_PASSWORD 环境变量中配置的凭据登录。</div>
                <div class="field"><label for="username">用户名</label><input id="username" autocomplete="username" placeholder="输入用户名"></div>
                <div class="field"><label for="password">密码</label><input id="password" type="password" autocomplete="current-password" placeholder="输入密码"></div>
                <button class="btn btn-primary" id="loginBtn" style="width:100%;justify-content:center">登录</button>
                <div class="login-status"><span class="badge" id="heroStatus2"><span class="dot"></span>未登录</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <!-- Dashboard -->
      <div id="dashboardScreen" class="hidden">
        <!-- Page Header -->
        <div class="page-header">
          <h1>Dashboard</h1>
          <p>系统运行状态总览与全局配置管理。所有改动即时同步至运行时内存。</p>
        </div>
        <!-- Stats Row -->
        <div class="card-grid quarter section">
          <div class="stat-card"><div class="stat-label">API Keys</div><div class="stat-value" id="statAuth">0</div><div class="stat-desc">本地认证密钥</div></div>
          <div class="stat-card"><div class="stat-label">直传前缀</div><div class="stat-value" id="statPrefixes">0</div><div class="stat-desc">Access Token 前缀</div></div>
          <div class="stat-card"><div class="stat-label">上游账户</div><div class="stat-value" id="statAccounts">0</div><div class="stat-desc">ChatGPT 账号池</div></div>
          <div class="stat-card"><div class="stat-label">可写配置</div><div class="stat-value" id="statWritable">-</div><div class="stat-desc">配置文件写入权限</div></div>
        </div>
        <!-- Config & Models Section -->
        <div class="section">
          <div class="card">
            <div class="card-body">
              <div class="card-header">
                <div>
                  <h3>运行状态</h3>
                  <p>配置绑定路径与运行时监听地址。改动写回 YAML 文件并即时刷新内存。（Render 无持久磁盘时重建后可能丢失）</p>
                </div>
                <div class="card-actions">
                  <span class="badge" id="saveStatus">等待加载</span>
                </div>
              </div>
              <div class="field-group">
                <div class="field"><label>Config Path</label><div style="font-family:var(--mono);font-size:13px;padding:9px 12px;background:var(--bg);border:1px solid var(--border-light);border-radius:var(--radius);color:var(--text-secondary);word-break:break-all" id="configPath">未加载</div></div>
                <div class="field"><label>Runtime Bind</label><div style="font-family:var(--mono);font-size:13px;padding:9px 12px;background:var(--bg);border:1px solid var(--border-light);border-radius:var(--radius);color:var(--text-secondary)" id="runtimeBind">未加载</div></div>
              </div>
              <div class="alert">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
                <span>这里会显示和保存真实 token。只在可信网络和 HTTPS 下使用，后台密码必须足够强。</span>
              </div>
            </div>
          </div>
        </div>
        <!-- Models Summary -->
        <div class="section">
          <div class="card">
            <div class="card-body">
              <div class="card-header">
                <div>
                  <h3>模型汇总</h3>
                  <p>每个上游账户独立探测模型后，可勾选要启用的模型。/v1/models 仅返回所有账户已勾选模型的去重汇总。</p>
                </div>
                <div class="card-actions">
                  <span class="badge" id="modelsStatus"><span class="dot"></span>尚未加载</span>
                </div>
              </div>
              <div class="row-list" id="modelsList"></div>
            </div>
          </div>
        </div>
        <!-- API Keys & Prefixes 2-col -->
        <div class="card-grid half section">
          <div class="card">
            <div class="card-body">
              <div class="card-header">
                <div><h3>API Keys</h3><p>用于调用 /v1/* 接口。建议至少保留一个强随机 Key。</p></div>
                <div class="card-actions">
                  <button class="btn btn-secondary btn-sm" id="generateAuthBtn">随机生成</button>
                  <button class="btn btn-secondary btn-sm" id="addAuthBtn">新增</button>
                </div>
              </div>
              <div class="row-list" id="authTokenList"></div>
            </div>
          </div>
          <div class="card">
            <div class="card-body">
              <div class="card-header">
                <div><h3>直传前缀</h3><p>配置后可通过 Authorization 直传上游 Access Token。</p></div>
                <div class="card-actions">
                  <button class="btn btn-secondary btn-sm" id="addPrefixBtn">新增</button>
                </div>
              </div>
              <div class="row-list" id="prefixList"></div>
            </div>
          </div>
        </div>
        <!-- Global Settings -->
        <div class="section">
          <div class="card">
            <div class="card-body">
              <div class="card-header">
                <div><h3>全局设置</h3><p>业务连接配置。轮询模式按优先级从小到大依次尝试账户。</p></div>
              </div>
              <div class="field-group">
                <div class="field"><label for="globalProxy">Proxy</label><input id="globalProxy" placeholder="http://127.0.0.1:7890"></div>
                <div class="field"><label for="globalBaseUrl">ChatGPT Base URL</label><input id="globalBaseUrl" placeholder="https://chatgpt.com"></div>
                <div class="field"><label for="accountRoutingMode">上游账户模式</label><select id="accountRoutingMode"><option value="round_robin">轮询</option><option value="single">单一账户</option></select></div>
                <div class="field"><label for="selectedAccount">单一账户选择</label><select id="selectedAccount"><option value="">请选择账户</option></select></div>
              </div>
            </div>
          </div>
        </div>
        <!-- Accounts -->
        <div class="section">
          <div class="card">
            <div class="card-body">
              <div class="card-header">
                <div><h3>上游账户池</h3><p>逐条增删改。access_token 为必填。获取：登录 chatgpt.com → /api/auth/session → 复制 accessToken，或直接导入该 JSON。</p></div>
                <div class="card-actions">
                  <button class="btn btn-secondary btn-sm" id="importBtn">批量导入</button>
                  <button class="btn btn-secondary btn-sm" id="addAccountBtn">新增账户</button>
                </div>
              </div>
              <!-- Collapsible batch import -->
              <div class="collapse-toggle" id="bulkTgl">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>
                批量导入 Token
              </div>
              <div class="collapse-content" id="bulkCollapse">
                <div class="field" style="margin-top:8px;margin-bottom:14px">
                  <textarea id="bulkTokens" placeholder="每行一个 token，也支持 token,proxy,email,type；还可以直接粘贴 /api/auth/session 的完整 JSON"></textarea>
                </div>
              </div>
              <div class="row-list account-list" id="accountList"></div>
            </div>
          </div>
        </div>
        <!-- Action bar -->
        <div style="display:flex;align-items:center;gap:8px;padding:16px 0;border-top:1px solid var(--border-light);margin-top:8px">
          <button class="btn btn-primary" id="saveBtn">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg>
            保存全部配置
          </button>
          <button class="btn btn-secondary" id="exportBtn">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
            导出配置
          </button>
          <label class="btn btn-secondary" for="importFile">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
            导入配置 / auth.txt
          </label>
          <input id="importFile" type="file" accept=".yaml,.yml,.json,.txt" class="hidden">
        </div>
      </div>
    </div>
  </div>
</div>
<div class="toast-container" id="toastContainer"></div>
<script>
// ====== Util ======
const $ = id => document.getElementById(id);
const toastContainer = $('toastContainer');
function toast(msg, kind) {
  const t = document.createElement('div');
  t.className = 'toast' + (kind ? ' ' + kind : '');
  t.textContent = msg;
  toastContainer.appendChild(t);
  setTimeout(() => { t.style.opacity = '0'; t.style.transition = 'opacity 0.2s'; setTimeout(() => t.remove(), 200); }, 3000);
}

// ====== DOM refs ======
const loginScreen = $('loginScreen');
const dashboardScreen = $('dashboardScreen');
const heroStatus = $('heroStatus');
const heroStatus2 = $('heroStatus2');
const saveStatus = $('saveStatus');
const modelsStatus = $('modelsStatus');
const authTokenList = $('authTokenList');
const prefixList = $('prefixList');
const accountList = $('accountList');
const bulkTokens = $('bulkTokens');
const bulkTgl = $('bulkTgl');
const bulkCollapse = $('bulkCollapse');
const globalProxy = $('globalProxy');
const globalBaseUrl = $('globalBaseUrl');
const accountRoutingMode = $('accountRoutingMode');
const selectedAccount = $('selectedAccount');
const modelsList = $('modelsList');
const statAuth = $('statAuth');
const statPrefixes = $('statPrefixes');
const statAccounts = $('statAccounts');
const statWritable = $('statWritable');
const configPath = $('configPath');
const runtimeBind = $('runtimeBind');

// Collapse toggle
let bulkOpen = false;
bulkTgl.onclick = () => {
  bulkOpen = !bulkOpen;
  bulkCollapse.classList.toggle('hidden', !bulkOpen);
  bulkTgl.classList.toggle('collapsed', !bulkOpen);
};

// ====== Status helpers ======
function setHero(text, kind) {
  const html = '<span class="dot"></span>' + text;
  const cls = 'badge' + (kind ? ' ' + kind : '');
  heroStatus.innerHTML = html; heroStatus.className = cls;
  heroStatus2.innerHTML = html; heroStatus2.className = cls;
}
function setSave(text, kind) {
  saveStatus.innerHTML = '<span class="dot"></span>' + text;
  saveStatus.className = 'badge' + (kind ? ' ' + kind : '');
}
function setModels(text, kind) {
  modelsStatus.innerHTML = '<span class="dot"></span>' + text;
  modelsStatus.className = 'badge' + (kind ? ' ' + kind : '');
}

// ====== API ======
async function parseResponse(r) {
  const d = await r.json().catch(() => ({}));
  if (!r.ok) {
    const detail = d && d.detail;
    throw new Error(detail && detail.msg ? String(detail.msg) : '请求失败: ' + r.status);
  }
  if (d && d.code && d.code !== 0) throw new Error(d.detail || d.message || '接口返回失败');
  return d.data;
}

// ====== Login / Logout ======
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
    loginScreen.classList.add('hidden');
    dashboardScreen.classList.remove('hidden');
    await loadConfig();
    await loadModels();
  } catch (e) {
    setHero(e.message, 'warn');
    toast(e.message, 'danger');
  }
}
async function logout() {
  await fetch('/admin/api/logout', { method: 'POST', credentials: 'same-origin' }).catch(() => {});
  dashboardScreen.classList.add('hidden');
  loginScreen.classList.remove('hidden');
  clearNode(modelsList);
  modelsList.appendChild(empty('登录后可查看模型探测结果。'));
  setModels('已退出');
  setHero('已退出');
  toast('已退出登录', 'warn');
}

// ====== DOM helpers ======
function clearNode(n) { while (n.firstChild) n.removeChild(n.firstChild); }
function empty(t) { const d = document.createElement('div'); d.className = 'empty'; d.textContent = t; return d; }
function normalize() { [authTokenList, prefixList, accountList].forEach(n => Array.from(n.querySelectorAll('.empty')).forEach(i => i.remove())); }
function ensureEmpty() {
  if (!authTokenList.children.length) authTokenList.appendChild(empty('当前没有 API Key。'));
  if (!prefixList.children.length) prefixList.appendChild(empty('当前没有直传前缀。'));
  if (!accountList.children.length) accountList.appendChild(empty('当前没有上游账户。'));
}

// ====== Simple row (key/prefix) ======
function simpleRow(v, p) {
  const w = document.createElement('div');
  w.className = 'row-item';
  const kf = document.createElement('div');
  kf.className = 'key-field';
  const ik = document.createElement('span');
  ik.className = 'key-icon';
  ik.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>';
  const i = document.createElement('input');
  i.value = v || ''; i.placeholder = p; i.spellcheck = false;
  const b = document.createElement('button');
  b.className = 'btn btn-ghost btn-xs';
  b.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>';
  b.title = '删除';
  b.onclick = () => { w.remove(); refresh(); ensureEmpty(); };
  kf.append(ik, i);
  w.append(kf, b);
  return w;
}
function addAuth(v) { normalize(); authTokenList.appendChild(simpleRow(v, 'sk-your-local-key')); refresh(); }
function addPrefix(v) { normalize(); prefixList.appendChild(simpleRow(v, 'your-private-prefix-')); refresh(); }

// ====== Field builder ======
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
  f.style.flexDirection = 'row';
  f.style.alignItems = 'center';
  f.style.gap = '8px';
  const l = document.createElement('label');
  l.textContent = label;
  l.style.fontSize = '12px';
  l.style.fontWeight = '600';
  l.style.color = 'var(--text-secondary)';
  const i = document.createElement('input');
  i.type = 'checkbox'; i.dataset.name = name;
  i.checked = !!checked;
  f.append(i, l);
  return f;
}

// ====== Account helpers ======
function hasAccountContent(account) {
  return !!(String(account.access_token||'').trim()||String(account.proxy||'').trim()||String(account.email||'').trim()||String(account.type||'').trim()||String(account.account_id||'').trim()||String(account.id_token||'').trim()||String(account.refresh_token||'').trim()||String(account.last_refresh||'').trim()||String(account.expired||'').trim());
}
function updateSelectedAccountOptions(preferred) {
  const current = preferred != null ? String(preferred) : selectedAccount.value;
  clearNode(selectedAccount);
  const eo = document.createElement('option'); eo.value = ''; eo.textContent = '请选择账户';
  selectedAccount.appendChild(eo);
  accounts().forEach((a, i) => {
    const opt = document.createElement('option');
    opt.value = a.id || a.email || a.account_id || a.access_token;
    opt.textContent = (a.email||a.account_id||('账户'+String(i+1))) + ' | ' + (a.enabled?'启用':'停用') + ' | 优先级 ' + String(a.priority||0);
    selectedAccount.appendChild(opt);
  });
  selectedAccount.value = current && Array.from(selectedAccount.options).some(o=>o.value===current) ? current : '';
  selectedAccount.disabled = accountRoutingMode.value !== 'single';
}
function renderAccountModels(container, account) {
  container.innerHTML = '';
  const available = (account.available_models || []);
  if (!available.length) { container.appendChild(empty('先点击"探测模型"获取可用模型。')); return; }
  const selected = new Set(account.selected_models || []);
  available.forEach(model => {
    const chip = document.createElement('label');
    chip.className = 'model-chip' + (selected.has(model) ? ' active' : '');
    const box = document.createElement('input');
    box.type = 'checkbox'; box.dataset.modelId = model; box.checked = selected.has(model);
    box.onchange = () => {
      chip.classList.toggle('active', box.checked);
      const card = chip.closest('.account-card');
      const cur = readAccountCard(card);
      const wrap = card.querySelector('[data-role="models-wrap"]');
      cur.selected_models = Array.from(wrap.querySelectorAll('input[data-model-id]:checked')).map(i=>i.dataset.modelId);
      writeAccountModels(card, cur);
      refresh(); updateSelectedAccountOptions(); refreshSummaryModels();
    };
    const span = document.createElement('span'); span.textContent = model;
    chip.append(box, span); container.appendChild(chip);
  });
}
async function probeAccountModels(card) {
  const account = readAccountCard(card);
  const status = card.querySelector('[data-role="probe-status"]');
  const modelsWrap = card.querySelector('[data-role="models-wrap"]');
  status.textContent = '探测中...'; status.className = 'badge';
  try {
    const r = await fetch('/admin/api/models/probe', {
      method: 'POST', headers: {'Content-Type':'application/json'}, credentials: 'same-origin',
      body: JSON.stringify({account_id: account.id||account.email||account.account_id, access_token: account.access_token, proxy: account.proxy})
    });
    const data = await parseResponse(r);
    account.available_models = data.available_models||[]; account.selected_models = data.available_models||[];
    writeAccountModels(card, account); renderAccountModels(modelsWrap, account);
    status.innerHTML = '<span class="dot"></span>已探测 ' + String(data.available||0) + ' / ' + String(data.total||0) + (data.source==='chatgpt_web_models'?' | 网页模型列表':' | 回退候选探测');
    status.className = 'badge ok'; toast('模型探测完成: '+String(data.available||0)+' 个可用', 'ok');
    refreshSummaryModels();
  } catch(e) { status.textContent = e.message; status.className = 'badge warn'; toast(e.message, 'danger'); }
}
function writeAccountModels(card, account) {
  card.dataset.availableModels = JSON.stringify(account.available_models||[]);
  card.dataset.selectedModels = JSON.stringify(account.selected_models||[]);
}
function readAccountModels(card) { return {available_models: JSON.parse(card.dataset.availableModels||'[]'), selected_models: JSON.parse(card.dataset.selectedModels||'[]')}; }
function readAccountCard(card) {
  const d = {};
  card.querySelectorAll('[data-name]').forEach(i => {
    if (i.type === 'checkbox') { d[i.dataset.name] = i.checked; return; }
    d[i.dataset.name] = i.value.trim();
  });
  d.priority = Number.parseInt(d.priority||'0',10); if(Number.isNaN(d.priority)) d.priority=0;
  const models = readAccountModels(card); d.available_models = models.available_models; d.selected_models = models.selected_models;
  return d;
}
function accountCard(a) {
  a = a||{};
  const c = document.createElement('div'); c.className = 'account-card';
  writeAccountModels(c, a);
  const h = document.createElement('div'); h.className = 'account-head';
  const t = document.createElement('div'); t.className = 'account-title';
  t.textContent = a.email||a.account_id||a.id||'新账户';
  const tools = document.createElement('div'); tools.className = 'card-actions';
  const probe = document.createElement('button'); probe.className = 'btn btn-secondary btn-xs';
  probe.innerHTML = '探测模型'; probe.onclick = () => probeAccountModels(c);
  const rm = document.createElement('button'); rm.className = 'btn btn-ghost btn-xs';
  rm.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>';
  rm.title = '删除';
  rm.onclick = () => { c.remove(); refresh(); ensureEmpty(); updateSelectedAccountOptions(); refreshSummaryModels(); };
  tools.append(probe, rm); h.append(t, tools);
  const g = document.createElement('div'); g.className = 'account-body';
  const fg = document.createElement('div'); fg.className = 'account-fields';
  fg.appendChild(field('Selector ID','id',a.id,'auto_or_custom_id'));
  fg.appendChild(checkboxField('Enabled','enabled',a.enabled!==false));
  fg.appendChild(field('Priority','priority',a.priority||0,'0','number'));
  [['Access Token','access_token','real_access_token'],['Proxy','proxy','http://127.0.0.1:7890'],['Email','email','user@example.com'],['Type','type','codex'],['Account ID','account_id','optional_id'],['ID Token','id_token','optional_id_token'],['Refresh Token','refresh_token','optional_refresh'],['Expired','expired',''],['Last Refresh','last_refresh','']].forEach(x => fg.appendChild(field(x[0],x[1],a[x[1]],x[2])));
  const mb = document.createElement('div'); mb.className = 'field';
  const ml = document.createElement('label'); ml.textContent = '模型';
  const ms = document.createElement('div'); ms.dataset.role = 'probe-status'; ms.className = 'badge';
  ms.textContent = (a.available_models&&a.available_models.length) ? '已加载' : '尚未探测';
  const mw = document.createElement('div'); mw.dataset.role = 'models-wrap'; mw.className = 'models-wrap';
  renderAccountModels(mw, a);
  mb.append(ml, ms, mw);
  const mbW = document.createElement('div'); mbW.style.gridColumn = '1 / -1'; mbW.appendChild(mb);
  fg.appendChild(mbW);
  g.appendChild(fg);
  g.oninput = () => {
    const email = fg.querySelector('input[data-name="email"]')?.value.trim()||'';
    const token = fg.querySelector('input[data-name="access_token"]')?.value.trim()||'';
    const idv = fg.querySelector('input[data-name="id"]')?.value.trim()||'';
    t.textContent = email||idv||token||'新账户';
    refresh(); updateSelectedAccountOptions(); refreshSummaryModels();
  };
  c.append(h, g);
  return c;
}
function addAccount(a) { normalize(); accountList.appendChild(accountCard(a)); refresh(); updateSelectedAccountOptions(); refreshSummaryModels(); }
function sessionAccountFromJSON(raw) {
  const s = typeof raw === 'string' ? JSON.parse(raw) : raw;
  const accessToken = String(s.accessToken||'').replace(/^Bearer\s+/i,'').trim();
  if(!accessToken) throw new Error('未找到 accessToken');
  const accountId = String((s.account&&s.account.id)||'').trim();
  const userId = String((s.user&&s.user.id)||'').trim();
  const iat = Number(s.user&&s.user.iat);
  return {
    id: accountId || userId,
    enabled: true,
    priority: 0,
    access_token: accessToken,
    email: String((s.user&&s.user.email)||'').trim(),
    type: String((s.account&&s.account.planType)||s.authProvider||'chatgpt-web').trim(),
    account_id: accountId,
    expired: String(s.expires||'').trim(),
    last_refresh: Number.isFinite(iat) && iat > 0 ? new Date(iat*1000).toISOString() : new Date().toISOString(),
    available_models: [],
    selected_models: []
  };
}
function sameAccount(a, b) {
  const av = String(a.account_id||'').trim(), bv = String(b.account_id||'').trim();
  if(av && bv && av === bv) return true;
  const ai = String(a.id||'').trim(), bi = String(b.id||'').trim();
  if(ai && bi && ai === bi) return true;
  const ae = String(a.email||'').trim().toLowerCase(), be = String(b.email||'').trim().toLowerCase();
  if(ae && be && ae === be) return true;
  const at = String(a.access_token||'').trim(), bt = String(b.access_token||'').trim();
  return !!(at && bt && at === bt);
}
function mergeImportedAccount(existing, incoming) {
  const out = Object.assign({}, existing);
  ['id','access_token','proxy','email','type','account_id','expired','last_refresh'].forEach(k => { if(String(incoming[k]||'').trim()) out[k] = incoming[k]; });
  if (out.enabled == null) out.enabled = incoming.enabled !== false;
  return out;
}
function addOrUpdateAccount(a) {
  normalize();
  for (const card of Array.from(accountList.children).filter(i=>!i.classList.contains('empty'))) {
    const current = readAccountCard(card);
    if (!sameAccount(current, a)) continue;
    card.replaceWith(accountCard(mergeImportedAccount(current, a)));
    refresh(); updateSelectedAccountOptions(); refreshSummaryModels();
    return 'updated';
  }
  addAccount(a);
  return 'added';
}

// ====== Data helpers ======
function listValues(n) { return Array.from(n.children).filter(i=>!i.classList.contains('empty')).map(i=>i.querySelector('input')).filter(Boolean).map(i=>i.value.trim()).filter(Boolean); }
function accounts() { return Array.from(accountList.children).filter(i=>!i.classList.contains('empty')).map(readAccountCard).filter(hasAccountContent); }
function refresh() {
  statAuth.textContent = listValues(authTokenList).length;
  statPrefixes.textContent = listValues(prefixList).length;
  statAccounts.textContent = accounts().length;
}
function refreshSummaryModels() {
  const seen = new Set(), summary = [];
  accounts().filter(a=>a.enabled!==false).forEach(a => (a.selected_models||[]).forEach(m => { if(!seen.has(m)){seen.add(m);summary.push(m)} }));
  renderModels({summary_models: summary});
}

// ====== Render ======
function render(p) {
  clearNode(authTokenList); clearNode(prefixList); clearNode(accountList);
  (p.auth_tokens||[]).forEach(addAuth); (p.access_token_prefixes||[]).forEach(addPrefix);
  (p.chatgpt_accounts||[]).forEach(addAccount);
  globalProxy.value = p.proxy||''; globalBaseUrl.value = p.chatgpt_base_url||'';
  accountRoutingMode.value = p.account_routing_mode||'round_robin';
  configPath.textContent = p.config_path||'当前运行态未绑定可写配置文件';
  runtimeBind.textContent = (p.runtime_bind||'-')+':'+String(p.runtime_port||'-');
  statWritable.textContent = p.writable?'可写':'只读';
  statWritable.style.color = p.writable?'var(--success)':'var(--text-tertiary)';
  refresh(); ensureEmpty(); updateSelectedAccountOptions(p.selected_account||'');
  renderModels({summary_models: p.summary_models||[]});
  setSave(p.writable?'配置已加载，可保存':'当前运行态只读，不可保存', p.writable?'ok':'warn');
}
function renderModels(payload) {
  clearNode(modelsList);
  const models = (payload.summary_models||[]);
  if (!models.length) { modelsList.appendChild(empty('当前没有已启用的汇总模型。')); setModels('尚未选择模型','warn'); return; }
  models.forEach(model => {
    const row = document.createElement('div'); row.className = 'row-item';
    const left = document.createElement('div');
    left.innerHTML = '<div style="font-weight:600;font-size:13px">'+model+'</div><div style="font-size:11px;color:var(--text-tertiary);margin-top:1px">/v1/models 将返回该模型</div>';
    const badge = document.createElement('span'); badge.className = 'badge ok';
    badge.innerHTML = '<span class="dot"></span>已汇总';
    row.append(left, badge); modelsList.appendChild(row);
  });
  setModels('当前汇总 '+String(models.length)+' 个模型','ok');
}

// ====== Network ======
async function loadConfig() {
  try {
    const r = await fetch('/admin/api/config', {credentials: 'same-origin'});
    render(await parseResponse(r));
    loginScreen.classList.add('hidden'); dashboardScreen.classList.remove('hidden');
    setHero('已登录','ok');
  } catch(e) {
    if (String(e.message).includes('401')) { dashboardScreen.classList.add('hidden'); loginScreen.classList.remove('hidden'); setHero('未登录'); setSave('登录后可加载配置','warn'); return; }
    setSave(e.message,'warn'); toast(e.message,'danger');
  }
}
async function loadModels() {
  try { const r = await fetch('/admin/api/models', {credentials: 'same-origin'}); renderModels(await parseResponse(r)); }
  catch(e) { if(String(e.message).includes('401')){setModels('登录后可查看模型汇总','warn');return} setModels(e.message,'warn'); }
}
async function saveConfig() {
  const payload = {proxy:globalProxy.value.trim(), chatgpt_base_url:globalBaseUrl.value.trim(), account_routing_mode:accountRoutingMode.value, selected_account:selectedAccount.value, auth_tokens:listValues(authTokenList), access_token_prefixes:listValues(prefixList), chatgpt_accounts:accounts()};
  setSave('正在保存...');
  try {
    const r = await fetch('/admin/api/config', {method:'PUT', headers:{'Content-Type':'application/json'}, credentials:'same-origin', body:JSON.stringify(payload)});
    await parseResponse(r); setSave('配置已保存并刷新运行时内存','ok'); toast('配置已保存','ok'); await loadConfig();
  } catch(e) { setSave(e.message,'warn'); toast(e.message,'danger'); }
}
async function generateAuthToken() {
  setSave('正在生成...');
  try {
    const r = await fetch('/admin/api/auth-token/generate', {method:'POST', credentials:'same-origin'});
    const data = await parseResponse(r); if(!data||!data.token) throw new Error('生成失败');
    addAuth(data.token); setSave('已生成随机 Key，记得保存','ok'); toast('已生成随机 Key','ok');
  } catch(e) { setSave(e.message,'warn'); toast(e.message,'danger'); }
}
async function exportConfig() {
  setSave('正在导出...');
  try {
    const r = await fetch('/admin/api/config/export', {credentials:'same-origin'});
    if(!r.ok) throw new Error('导出失败: '+r.status);
    const blob = await r.blob(); const url = URL.createObjectURL(blob);
    const a = document.createElement('a'); a.href = url; a.download = 'chat2api-config.yaml';
    document.body.appendChild(a); a.click(); a.remove(); URL.revokeObjectURL(url);
    setSave('配置已导出','ok'); toast('配置已导出','ok');
  } catch(e) { setSave(e.message,'warn'); toast(e.message,'danger'); }
}
async function importConfig(file) {
  if(!file) return;
  const form = new FormData(); form.append('file', file);
  setSave('正在导入...');
  try {
    const r = await fetch('/admin/api/config/import', {method:'POST', credentials:'same-origin', body:form});
    await parseResponse(r); setSave('配置已导入并刷新运行时内存','ok'); toast('配置已导入','ok');
    await loadConfig(); await loadModels();
  } catch(e) { setSave(e.message,'warn'); toast(e.message,'danger'); }
}
function importBulk() {
  const raw = bulkTokens.value.trim();
  if(!raw) { toast('批量导入内容为空','warn'); return; }
  if(raw.startsWith('{')) {
    try {
      const action = addOrUpdateAccount(sessionAccountFromJSON(raw));
      bulkTokens.value = ''; toast(action === 'updated' ? '已从 session JSON 更新账户，记得保存' : '已从 session JSON 填入账户，记得保存','ok');
    } catch(e) { toast('session JSON 解析失败: '+e.message,'danger'); }
    return;
  }
  let count = 0;
  raw.split(/\n+/).forEach(line => { const parts = line.trim().split(',').map(x=>x.trim()); if(parts[0]) { addOrUpdateAccount({access_token:parts[0], proxy:parts[1]||'', email:parts[2]||'', type:parts[3]||'', enabled:true, priority:0}); count++; } });
  bulkTokens.value = ''; toast('已加入/更新 '+String(count)+' 个账户，记得保存','ok');
}

// ====== Navigation: sidebar smooth scroll ======
function scrollToSection(id) {
  const el = document.getElementById(id);
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
}
// Highlight active nav item
const navItems = ['navDashboard','navAccounts','navKeys','navSettings'];
function setActiveNav(activeId) {
  navItems.forEach(id => {
    const el = document.getElementById(id);
    if (el) el.classList.toggle('active', id === activeId);
  });
}
// Observe sections for scroll-based nav highlight
const navSectionMap = {
  navDashboard: 'dashboardScreen',
  navAccounts: 'accountList',
  navKeys: 'authTokenList',
  navSettings: 'globalProxy'
};
// IntersectionObserver for active nav
if (typeof IntersectionObserver !== 'undefined') {
  const sectionEls = Object.values(navSectionMap).map(id => document.getElementById(id)).filter(Boolean);
  const navObs = new IntersectionObserver(entries => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        for (const [navId, secId] of Object.entries(navSectionMap)) {
          if (secId === entry.target.id) { setActiveNav(navId); break; }
        }
      }
    });
  }, { rootMargin: '-80px 0px -60% 0px' });
  sectionEls.forEach(el => navObs.observe(el));
}
$('navDashboard').onclick = () => { scrollToSection('dashboardScreen'); setActiveNav('navDashboard'); };
$('navAccounts').onclick = () => { scrollToSection('accountList'); setActiveNav('navAccounts'); };
$('navKeys').onclick = () => { scrollToSection('authTokenList'); setActiveNav('navKeys'); };
$('navSettings').onclick = () => { scrollToSection('globalProxy'); setActiveNav('navSettings'); };

// ====== Events ======
accountRoutingMode.onchange = () => updateSelectedAccountOptions();
$('loginBtn').onclick = login;
$('password').addEventListener('keydown', e => { if(e.key==='Enter') login(); });
$('navLogout').onclick = logout;
$('saveBtn').onclick = saveConfig;
$('exportBtn').onclick = exportConfig;
$('importFile').onchange = (e) => { const file = e.target.files&&e.target.files[0]; importConfig(file).finally(()=>{e.target.value=''}); };
$('generateAuthBtn').onclick = generateAuthToken;
$('addAuthBtn').onclick = () => addAuth('');
$('addPrefixBtn').onclick = () => addPrefix('');
$('addAccountBtn').onclick = () => addAccount({enabled:true, priority:0, available_models:[], selected_models:[]});
$('importBtn').onclick = importBulk;

// ====== Init ======
loadConfig(); loadModels(); ensureEmpty(); updateSelectedAccountOptions();
</script>
</body>
</html>`
