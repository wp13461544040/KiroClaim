// JWT 认证模块
var ADMIN_TOKEN = localStorage.getItem('adminToken');

// 检查登录状态，决定显示登录页还是后台
async function checkAuth() {
  var loginPage = document.getElementById('loginPage');
  var appContainer = document.getElementById('appContainer');
  if (!loginPage || !appContainer) return;

  var initialized = await checkAdminInitialized();
  if (initialized === false) {
    localStorage.removeItem('adminToken');
    ADMIN_TOKEN = null;
    location.replace('/setup');
    return;
  }

  if (ADMIN_TOKEN) {
    // 有缓存的 token，验证是否有效
    var valid = await verifyToken(ADMIN_TOKEN);
    if (valid) {
      showApp();
      return;
    }
    // token 失效，清除
    localStorage.removeItem('adminToken');
    ADMIN_TOKEN = null;
  }
  showLogin();
}

// 检查是否已创建初始管理员账号
async function checkAdminInitialized() {
  try {
    var r = await fetch('/admin/setup/status', { method: 'GET' });
    if (!r.ok) return null;
    var result = await r.json();
    if (result.code !== 0 || !result.data) return null;
    return !!result.data.initialized;
  } catch (e) {
    return null;
  }
}

// 验证 token 是否有效（轻量 ping，不查 DB）
async function verifyToken(token) {
  try {
    var r = await fetch('/admin/me', {
      method: 'GET',
      headers: { 'Authorization': 'Bearer ' + token }
    });
    return r.status === 200;
  } catch (e) {
    return false;
  }
}

// 显示登录页
function showLogin() {
  document.getElementById('loginPage').style.display = 'flex';
  document.getElementById('appContainer').style.display = 'none';
  var usernameInput = document.getElementById('loginUsernameInput');
  var passwordInput = document.getElementById('loginPasswordInput');
  if (usernameInput) {
    usernameInput.value = '';
    usernameInput.focus();
  }
  if (passwordInput) {
    passwordInput.value = '';
  }
}

// 显示后台
function showApp() {
  document.getElementById('loginPage').style.display = 'none';
  document.getElementById('appContainer').style.display = 'flex';
}

// 登录提交
async function doLogin() {
  var usernameInput = document.getElementById('loginUsernameInput');
  var passwordInput = document.getElementById('loginPasswordInput');
  var errorEl = document.getElementById('loginError');
  var btn = document.getElementById('loginBtn');
  if (!usernameInput || !passwordInput) return;

  var username = usernameInput.value.trim();
  var password = passwordInput.value.trim();

  if (!username || !password) {
    errorEl.textContent = '请输入用户名和密码';
    return;
  }

  btn.disabled = true;
  btn.textContent = '登录中...';
  errorEl.textContent = '';

  try {
    var response = await fetch('/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username, password: password })
    });

    var result = await response.json();

    if (result.code === 0 && result.data && result.data.token) {
      ADMIN_TOKEN = result.data.token;
      localStorage.setItem('adminToken', ADMIN_TOKEN);
      showApp();
      // 初始化后台页面
      initApp();
    } else {
      errorEl.textContent = result.message || '登录失败';
      passwordInput.value = '';
      passwordInput.focus();
    }
  } catch (e) {
    errorEl.textContent = '网络错误，请重试';
  }

  btn.disabled = false;
  btn.textContent = '登录';
}

// 退出登录
function logout() {
  localStorage.removeItem('adminToken');
  ADMIN_TOKEN = null;
  showLogin();
}
