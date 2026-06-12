package service

import (
	"chat2api/app/conf"
	"chat2api/app/result"
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

const adminPageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>chat2api Admin</title>
  <style>
    :root{--bg:#f4efe5;--panel:rgba(255,252,247,.9);--panel2:#fffaf1;--line:rgba(84,63,38,.16);--text:#1f1b16;--muted:#6f665d;--accent:#b95c2e;--deep:#8f3e17;--ok:#1f7a46;--warn:#9a5a00;--shadow:0 28px 70px rgba(69,48,25,.12);--mono:"IBM Plex Mono","Consolas",monospace;--sans:"IBM Plex Sans","Avenir Next","Segoe UI",sans-serif}
    *{box-sizing:border-box}body{margin:0;color:var(--text);font-family:var(--sans);background:radial-gradient(circle at top left,rgba(185,92,46,.18),transparent 34%),radial-gradient(circle at top right,rgba(86,134,95,.14),transparent 28%),linear-gradient(180deg,#f8f1e7 0%,#efe7db 100%);min-height:100vh}.shell{width:min(1240px,calc(100vw - 32px));margin:24px auto 48px;display:grid;gap:18px}.hero{background:linear-gradient(135deg,rgba(33,25,18,.96),rgba(76,44,24,.92));color:#fff7ed;border-radius:28px;padding:28px;box-shadow:var(--shadow);overflow:hidden;position:relative}.hero-grid{display:grid;grid-template-columns:1.35fr 1fr;gap:18px}.title{margin:0 0 8px;font-size:clamp(28px,5vw,44px);line-height:1.05;letter-spacing:-.03em}.subtitle{margin:0;max-width:58ch;color:rgba(255,247,237,.78);line-height:1.65}.login-panel{background:rgba(255,247,237,.08);border:1px solid rgba(255,247,237,.12);border-radius:20px;padding:18px;backdrop-filter:blur(10px)}h2{margin:0 0 12px;font-size:15px;letter-spacing:.08em;text-transform:uppercase}.grid{display:grid;gap:18px;grid-template-columns:repeat(12,1fr)}.section{grid-column:span 12;background:var(--panel);backdrop-filter:blur(14px);border:1px solid rgba(255,255,255,.46);border-radius:24px;box-shadow:var(--shadow);padding:22px}.section-header{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:16px}.section-copy{display:grid;gap:6px}.section-copy p,.login-panel p{margin:0;color:var(--muted);line-height:1.55;font-size:14px}.login-panel p{color:rgba(255,247,237,.76);margin-bottom:12px}.stats{display:grid;gap:12px;grid-template-columns:repeat(4,minmax(0,1fr))}.stat,.meta,.row-item,.account-card{background:var(--panel2);border:1px solid var(--line);border-radius:18px}.stat{padding:16px;display:grid;gap:6px}.label{color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.08em}.value{font-family:var(--mono);font-size:14px;line-height:1.6;word-break:break-all}.stat .value{font-family:var(--sans);font-size:28px;font-weight:800;letter-spacing:-.03em}.meta-grid{display:grid;gap:12px;grid-template-columns:repeat(2,minmax(0,1fr));margin-top:12px}.meta{padding:16px;display:grid;gap:6px}.row-list,.account-list{display:grid;gap:12px}.row-item{display:grid;grid-template-columns:1fr auto;gap:10px;align-items:center;padding:14px}.account-card{display:grid;gap:12px;padding:14px}.account-head{display:flex;align-items:center;justify-content:space-between;gap:12px}.account-title{font-weight:800}.account-grid{display:grid;gap:12px;grid-template-columns:repeat(2,minmax(0,1fr))}.field{display:grid;gap:7px}.field label{font-size:12px;color:var(--muted);text-transform:uppercase;letter-spacing:.08em}.field input,.field textarea{width:100%;border:1px solid rgba(84,63,38,.18);background:#fffdf8;color:var(--text);border-radius:12px;padding:11px 12px;outline:none;font-family:var(--mono);font-size:13px}.field textarea{min-height:132px;resize:vertical;line-height:1.6}.field input:focus,.field textarea:focus{border-color:rgba(185,92,46,.55);box-shadow:0 0 0 4px rgba(185,92,46,.12)}.toolbar{display:flex;flex-wrap:wrap;gap:10px;align-items:center}.btn{appearance:none;border:0;cursor:pointer;padding:11px 16px;border-radius:999px;font-weight:800;transition:transform .18s ease,opacity .18s ease}.btn:hover{transform:translateY(-1px)}.btn-primary{color:#fff7ed;background:linear-gradient(135deg,var(--accent),var(--deep));box-shadow:0 14px 28px rgba(143,62,23,.22)}.btn-secondary{color:var(--text);background:rgba(255,255,255,.88);border:1px solid var(--line)}.btn-danger{color:#7d1d1d;background:rgba(163,43,43,.08);border:1px solid rgba(163,43,43,.16)}.status{display:inline-flex;align-items:center;min-height:40px;border-radius:999px;padding:0 14px;background:rgba(255,255,255,.12);color:rgba(255,247,237,.92);font-size:13px}.status-inline{color:var(--muted);background:rgba(185,92,46,.12)}.status-inline.ok{color:var(--ok);background:rgba(31,122,70,.1)}.status-inline.warn{color:var(--warn);background:rgba(154,90,0,.12)}.note{margin:12px 0 0;padding:14px 16px;border-radius:16px;border:1px solid rgba(154,90,0,.18);background:rgba(255,196,89,.12);color:#6a4500;line-height:1.6;font-size:14px}.empty{padding:18px;border:1px dashed rgba(84,63,38,.24);border-radius:16px;color:var(--muted);text-align:center;background:rgba(255,255,255,.36)}.hidden{display:none!important}@media(max-width:980px){.hero-grid,.stats,.meta-grid,.account-grid{grid-template-columns:1fr}.section-header{align-items:flex-start;flex-direction:column}}
  </style>
</head>
<body>
  <div class="shell">
    <section class="hero"><div class="hero-grid"><div><h1 class="title">chat2api Admin</h1><p class="subtitle">Render 只需要配置后台账号密码。登录后台后，可以管理本地 API keys、直传前缀、上游 ChatGPT 账号池和全局代理配置。</p></div><div class="login-panel"><h2>后台登录</h2><p>使用 Render 环境变量 ADMIN_USERNAME 和 ADMIN_PASSWORD 登录。</p><div class="field"><label for="username">Username</label><input id="username" autocomplete="username" placeholder="admin"></div><div class="field" style="margin-top:10px"><label for="password">Password</label><input id="password" type="password" autocomplete="current-password" placeholder="password"></div><div class="toolbar" style="margin-top:12px"><button class="btn btn-primary" id="loginBtn">登录</button><button class="btn btn-secondary" id="logoutBtn">退出</button></div><div class="status" id="heroStatus" style="margin-top:12px">未登录</div></div></div></section>
    <main id="adminApp" class="hidden">
      <section class="section"><div class="section-header"><div class="section-copy"><h2>运行状态</h2><p>配置改动会写回当前 YAML 文件，并即时刷新内存中的 token 池。Render 如果没有挂持久磁盘，重建实例后文件改动可能丢失。</p></div><button class="btn btn-primary" id="saveBtn">保存全部配置</button></div><div class="stats"><div class="stat"><div class="label">本地 API Keys</div><div class="value" id="statAuth">0</div></div><div class="stat"><div class="label">直传前缀</div><div class="value" id="statPrefixes">0</div></div><div class="stat"><div class="label">上游账号</div><div class="value" id="statAccounts">0</div></div><div class="stat"><div class="label">可写配置</div><div class="value" id="statWritable">否</div></div></div><div class="meta-grid"><div class="meta"><div class="label">Config Path</div><div class="value" id="configPath">未加载</div></div><div class="meta"><div class="label">Runtime Bind</div><div class="value" id="runtimeBind">未加载</div></div></div><p class="note">高危提示：这里会显示和保存真实 token。只在可信网络和 HTTPS 下使用，后台密码必须足够强。</p></section>
      <div class="grid"><section class="section" style="grid-column:span 6"><div class="section-header"><div class="section-copy"><h2>本地 API Keys</h2><p>这些 key 用于调用 /v1/*。建议至少保留一个强随机 key。</p></div><button class="btn btn-secondary" id="addAuthBtn">新增 Key</button></div><div class="row-list" id="authTokenList"></div></section><section class="section" style="grid-column:span 6"><div class="section-header"><div class="section-copy"><h2>直传 Access Token 前缀</h2><p>配置后可用 Authorization: Bearer &lt;prefix&gt;&lt;real_access_token&gt; 直接请求上游。</p></div><button class="btn btn-secondary" id="addPrefixBtn">新增前缀</button></div><div class="row-list" id="prefixList"></div></section><section class="section"><div class="section-header"><div class="section-copy"><h2>全局设置</h2><p>这里只处理业务连接配置，不保存 Render 的 PORT/BIND 临时变量。</p></div><div class="status status-inline" id="saveStatus">等待加载</div></div><div class="account-grid"><div class="field"><label for="globalProxy">Proxy</label><input id="globalProxy" placeholder="http://127.0.0.1:7890"></div><div class="field"><label for="globalBaseUrl">ChatGPT Base URL</label><input id="globalBaseUrl" placeholder="https://chatgpt.com"></div></div></section><section class="section"><div class="section-header"><div class="section-copy"><h2>上游账号池</h2><p>支持逐条增删改。只有 access_token 是必要字段。</p></div><div class="toolbar"><button class="btn btn-secondary" id="addAccountBtn">新增账号</button><button class="btn btn-secondary" id="importBtn">导入多条 Token</button></div></div><div class="field" style="margin-bottom:12px"><label for="bulkTokens">批量导入</label><textarea id="bulkTokens" placeholder="每行一个 token；也支持 token,proxy,email,type 四列逗号格式"></textarea></div><div class="account-list" id="accountList"></div></section></div>
    </main>
  </div>
  <script>
    const $ = (id) => document.getElementById(id);
    const adminApp=$('adminApp'), heroStatus=$('heroStatus'), saveStatus=$('saveStatus'), authTokenList=$('authTokenList'), prefixList=$('prefixList'), accountList=$('accountList'), bulkTokens=$('bulkTokens'), globalProxy=$('globalProxy'), globalBaseUrl=$('globalBaseUrl');
    const statAuth=$('statAuth'), statPrefixes=$('statPrefixes'), statAccounts=$('statAccounts'), statWritable=$('statWritable'), configPath=$('configPath'), runtimeBind=$('runtimeBind');
    function setHero(text,kind){heroStatus.textContent=text;heroStatus.className='status';if(kind==='ok')heroStatus.classList.add('ok');if(kind==='warn')heroStatus.classList.add('warn')}
    function setSave(text,kind){saveStatus.textContent=text;saveStatus.className='status status-inline';if(kind==='ok')saveStatus.classList.add('ok');if(kind==='warn')saveStatus.classList.add('warn')}
    async function parseResponse(r){const d=await r.json().catch(()=>({}));if(!r.ok){const detail=d&&d.detail;throw new Error(detail&&detail.msg?String(detail.msg):'请求失败: '+r.status)}if(d&&d.code&&d.code!==0)throw new Error(d.detail||d.message||'接口返回失败');return d.data}
    async function login(){setHero('正在登录...');try{const r=await fetch('/admin/api/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:$('username').value.trim(),password:$('password').value})});await parseResponse(r);$('password').value='';setHero('已登录','ok');adminApp.classList.remove('hidden');await loadConfig()}catch(e){setHero(e.message,'warn')}}
    async function logout(){await fetch('/admin/api/logout',{method:'POST',credentials:'same-origin'}).catch(()=>{});adminApp.classList.add('hidden');setHero('已退出');}
    function clearNode(n){while(n.firstChild)n.removeChild(n.firstChild)}function empty(t){const d=document.createElement('div');d.className='empty';d.textContent=t;return d}function normalize(){[authTokenList,prefixList,accountList].forEach(n=>Array.from(n.querySelectorAll('.empty')).forEach(i=>i.remove()))}function ensureEmpty(){if(!authTokenList.children.length)authTokenList.appendChild(empty('当前没有本地 API key。'));if(!prefixList.children.length)prefixList.appendChild(empty('当前没有 access token 前缀。'));if(!accountList.children.length)accountList.appendChild(empty('当前没有上游账号。'))}
    function simpleRow(v,p){const w=document.createElement('div');w.className='row-item';const i=document.createElement('input');i.value=v||'';i.placeholder=p;i.style.width='100%';const b=document.createElement('button');b.className='btn btn-danger';b.textContent='删除';b.onclick=()=>{w.remove();refresh();ensureEmpty()};w.append(i,b);return w}function addAuth(v){normalize();authTokenList.appendChild(simpleRow(v,'sk-your-local-key'));refresh()}function addPrefix(v){normalize();prefixList.appendChild(simpleRow(v,'your-private-prefix-'));refresh()}
    function field(label,name,value,placeholder){const f=document.createElement('div');f.className='field';const l=document.createElement('label');l.textContent=label;const i=document.createElement('input');i.dataset.name=name;i.value=value||'';i.placeholder=placeholder||'';f.append(l,i);return f}
    function accountCard(a){a=a||{};const c=document.createElement('div');c.className='account-card';const h=document.createElement('div');h.className='account-head';const t=document.createElement('div');t.className='account-title';t.textContent=a.email||'新账号';const rm=document.createElement('button');rm.className='btn btn-danger';rm.textContent='删除';rm.onclick=()=>{c.remove();refresh();ensureEmpty()};h.append(t,rm);const g=document.createElement('div');g.className='account-grid';[['Access Token','access_token','real_access_token'],['Proxy','proxy','http://127.0.0.1:7890'],['Email','email','user@example.com'],['Type','type','codex'],['Account ID','account_id','optional_account_id'],['ID Token','id_token','optional_id_token'],['Refresh Token','refresh_token','optional_refresh_token'],['Expired','expired',''],['Last Refresh','last_refresh','']].forEach(x=>g.appendChild(field(x[0],x[1],a[x[1]],x[2])));g.oninput=()=>{const email=g.querySelector('input[data-name="email"]').value.trim();const token=g.querySelector('input[data-name="access_token"]').value.trim();t.textContent=email||token||'新账号'};c.append(h,g);return c}function addAccount(a){normalize();accountList.appendChild(accountCard(a));refresh()}
    function listValues(n){return Array.from(n.children).filter(i=>!i.classList.contains('empty')).map(i=>i.querySelector('input')).filter(Boolean).map(i=>i.value.trim()).filter(Boolean)}function accounts(){return Array.from(accountList.children).filter(i=>!i.classList.contains('empty')).map(card=>{const d={};card.querySelectorAll('input[data-name]').forEach(i=>d[i.dataset.name]=i.value.trim());return d}).filter(a=>Object.values(a).some(Boolean))}function refresh(){statAuth.textContent=listValues(authTokenList).length;statPrefixes.textContent=listValues(prefixList).length;statAccounts.textContent=accounts().length}
    function render(p){clearNode(authTokenList);clearNode(prefixList);clearNode(accountList);(p.auth_tokens||[]).forEach(addAuth);(p.access_token_prefixes||[]).forEach(addPrefix);(p.chatgpt_accounts||[]).forEach(addAccount);globalProxy.value=p.proxy||'';globalBaseUrl.value=p.chatgpt_base_url||'';configPath.textContent=p.config_path||'当前运行态未绑定可写配置文件';runtimeBind.textContent=(p.runtime_bind||'-')+':'+String(p.runtime_port||'-');statWritable.textContent=p.writable?'是':'否';refresh();ensureEmpty();setSave(p.writable?'配置已加载，可保存':'当前运行态只读，不可保存',p.writable?'ok':'warn')}
    async function loadConfig(){try{const r=await fetch('/admin/api/config',{credentials:'same-origin'});render(await parseResponse(r));adminApp.classList.remove('hidden');setHero('已登录','ok')}catch(e){setSave(e.message,'warn');if(String(e.message).includes('401'))adminApp.classList.add('hidden')}}
    async function saveConfig(){const payload={proxy:globalProxy.value.trim(),chatgpt_base_url:globalBaseUrl.value.trim(),auth_tokens:listValues(authTokenList),access_token_prefixes:listValues(prefixList),chatgpt_accounts:accounts()};setSave('正在保存配置...');try{const r=await fetch('/admin/api/config',{method:'PUT',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify(payload)});await parseResponse(r);setSave('配置已保存并刷新运行时内存','ok');await loadConfig()}catch(e){setSave(e.message,'warn')}}
    function importBulk(){const raw=bulkTokens.value.trim();if(!raw){setSave('批量导入内容为空','warn');return}raw.split(/\n+/).forEach(line=>{const parts=line.trim().split(',').map(x=>x.trim());if(parts[0])addAccount({access_token:parts[0],proxy:parts[1]||'',email:parts[2]||'',type:parts[3]||''})});bulkTokens.value='';setSave('批量 token 已加入待保存列表','ok')}
    $('loginBtn').onclick=login;$('logoutBtn').onclick=logout;$('saveBtn').onclick=saveConfig;$('addAuthBtn').onclick=()=>addAuth('');$('addPrefixBtn').onclick=()=>addPrefix('');$('addAccountBtn').onclick=()=>addAccount({});$('importBtn').onclick=importBulk;$('password').addEventListener('keydown',e=>{if(e.key==='Enter')login()});loadConfig();ensureEmpty();
  </script>
</body>
</html>`
