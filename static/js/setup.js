var setupEnvReady = false;
var setupSubmitting = false;
var setupChecking = false;

async function setupStatus() {
  try {
    var r = await fetch('/admin/setup/status', { method: 'GET' });
    if (!r.ok) return null;
    return await r.json();
  } catch (e) {
    return null;
  }
}

async function setupChecks() {
  try {
    var r = await fetch('/admin/setup/checks', { method: 'GET' });
    if (!r.ok) return null;
    return await r.json();
  } catch (e) {
    return null;
  }
}

function setSetupError(message) {
  var errorEl = document.getElementById('setupError');
  if (errorEl) errorEl.textContent = message || '';
}

function setSetupEnvError(message) {
  var errorEl = document.getElementById('setupEnvError');
  if (errorEl) errorEl.textContent = message || '';
}

function getAdminForm() {
  var usernameInput = document.getElementById('setupUsernameInput');
  var passwordInput = document.getElementById('setupPasswordInput');
  var confirmInput = document.getElementById('setupPasswordConfirmInput');
  return {
    usernameInput: usernameInput,
    passwordInput: passwordInput,
    confirmInput: confirmInput,
    username: usernameInput ? usernameInput.value.trim() : '',
    password: passwordInput ? passwordInput.value.trim() : '',
    confirmPassword: confirmInput ? confirmInput.value.trim() : ''
  };
}

function validateAdminForm(focusOnError) {
  var form = getAdminForm();
  if (!form.username || !form.password || !form.confirmPassword) {
    setSetupError('请填写用户名和密码');
    if (focusOnError && !form.username && form.usernameInput) form.usernameInput.focus();
    return false;
  }
  if (form.username.length < 3) {
    setSetupError('用户名至少需要 3 个字符');
    if (focusOnError && form.usernameInput) form.usernameInput.focus();
    return false;
  }
  if (form.password.length < 8) {
    setSetupError('密码至少需要 8 个字符');
    if (focusOnError && form.passwordInput) form.passwordInput.focus();
    return false;
  }
  if (form.password !== form.confirmPassword) {
    setSetupError('两次输入的密码不一致');
    if (focusOnError && form.confirmInput) form.confirmInput.focus();
    return false;
  }
  setSetupError('');
  return true;
}

function switchSetupStep(name) {
  var adminPanel = document.getElementById('adminStepPanel');
  var envPanel = document.getElementById('envStepPanel');
  var adminStep = document.getElementById('stepAdmin');
  var envStep = document.getElementById('stepEnv');
  var statePill = document.getElementById('setupStatePill');
  var isEnv = name === 'env';

  if (adminPanel) {
    adminPanel.classList.toggle('active', !isEnv);
    adminPanel.hidden = isEnv;
  }
  if (envPanel) {
    envPanel.classList.toggle('active', isEnv);
    envPanel.hidden = !isEnv;
  }
  if (adminStep) adminStep.classList.toggle('active', !isEnv);
  if (envStep) envStep.classList.toggle('active', isEnv);
  if (statePill) {
    statePill.textContent = isEnv ? '环境检测' : '账号创建';
    if (!isEnv) {
      statePill.classList.remove('ok', 'fail');
    }
  }
}

async function goEnvStep() {
  if (!validateAdminForm(true)) return;
  switchSetupStep('env');
  setSetupEnvError('');
  await refreshSetupChecks();
}

function goAdminStep() {
  switchSetupStep('admin');
  setSetupEnvError('');
  updateSetupButton();
  var usernameInput = document.getElementById('setupUsernameInput');
  if (usernameInput) usernameInput.focus();
}

function renderSetupChecks(checks, ready) {
  var body = document.getElementById('setupChecksBody');
  if (!body) return;

  if (!Array.isArray(checks) || checks.length === 0) {
    setupEnvReady = false;
    updateSetupHealth(0, 0, false);
    body.innerHTML = renderUnifiedCheckPanel('检测失败', '无法读取本地环境状态', [{
      title: '检测结果',
      subtitle: '请确认服务正常运行后重新检测',
      icon: 'database',
      items: [{
        key: 'check_failed',
        icon: 'database',
        ok: false,
        label: '检测失败',
        message: '无法读取本地环境状态'
      }]
    }], false);
    updateSetupButton();
    return;
  }

  var passed = checks.filter(function(item) { return !!item.ok; }).length;
  updateSetupHealth(passed, checks.length, !!ready);
  body.innerHTML = renderGroupedChecks(checks, !!ready);

  setupEnvReady = !!ready;
  updateSetupButton();
  setSetupEnvError(setupEnvReady ? '' : '本地环境检测未通过，修复本地配置后重启服务再重新检测');
}

function renderCheckingState() {
  var body = document.getElementById('setupChecksBody');
  if (body) {
    body.innerHTML = renderUnifiedCheckPanel('正在检测', '正在读取服务器、数据库和初始化状态', [{
      title: '本地环境',
      subtitle: '读取本地配置和数据库连接状态',
      icon: 'server',
      items: [{
        key: 'checking',
        icon: 'server',
        ok: true,
        pending: true,
        label: '正在检测',
        message: '正在读取本地配置和数据库连接状态'
      }]
    }], false, 'pending');
  }
  updateSetupHealth(0, 0, false, '检测中');
}

function updateSetupHealth(passed, total, ready, title) {
  var statePill = document.getElementById('setupStatePill');
  if (statePill) {
    statePill.textContent = ready ? '可以初始化' : title || '环境检测';
    statePill.classList.toggle('ok', !!ready);
    statePill.classList.toggle('fail', total > 0 && !ready);
  }
}

async function refreshSetupChecks() {
  var btn = document.getElementById('setupCheckBtn');
  setupChecking = true;
  setupEnvReady = false;
  updateSetupButton();
  renderCheckingState();
  setSetupEnvError('');
  if (btn) {
    btn.disabled = true;
    btn.textContent = '检测中...';
  }

  var result = await setupChecks();
  if (result && result.code === 0 && result.data) {
    renderSetupChecks(result.data.checks || [], result.data.ready);
  } else {
    renderSetupChecks([], false);
    setSetupEnvError('无法完成环境检测');
  }

  setupChecking = false;
  updateSetupButton();
  if (btn) {
    btn.disabled = false;
    btn.textContent = '重新检测';
  }
}

function updateSetupButton() {
  var btn = document.getElementById('setupBtn');
  if (!btn) return;
  btn.disabled = !setupEnvReady || setupSubmitting || setupChecking;
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function checkKeyClass(key) {
  return String(key || 'default').replace(/[^a-zA-Z0-9_-]/g, '_');
}

function checkIconName(item) {
  return item && item.icon ? item.icon : item && item.key ? item.key : 'default';
}

function renderGroupedChecks(checks, ready) {
  var serverItems = checks.filter(function(item) {
    return String(item.key || '').indexOf('server_') === 0;
  });
  var databaseItems = checks.filter(function(item) {
    return String(item.key || '').indexOf('database_') === 0;
  });

  return renderUnifiedCheckPanel(
    ready ? '本地环境正常' : '本地环境存在异常',
    ready ? '服务器与数据库配置已通过检测' : '请修复未通过的检测项后重新检测',
    [{
      title: '服务器配置',
      subtitle: '运行环境与监听配置',
      icon: 'server',
      items: serverItems
    }, {
      title: '数据库配置',
      subtitle: '数据库类型、路径和连接状态',
      icon: databaseIconFor(databaseItems),
      items: databaseItems
    }],
    ready
  );
}

function renderUnifiedCheckPanel(title, subtitle, groups, ready, state) {
  groups = (groups || []).filter(function(group) {
    return group && group.items && group.items.length;
  });
  var total = groups.reduce(function(sum, group) { return sum + group.items.length; }, 0);
  var passed = groups.reduce(function(sum, group) {
    return sum + group.items.filter(function(item) { return !!item.ok && !item.pending; }).length;
  }, 0);
  var panelState = state || (ready ? 'ok' : 'fail');
  var summary = panelState === 'pending' ? '检测中' : (passed + ' / ' + total + ' 通过');

  return '<section class="setup-check-panel ' + panelState + '">' +
    '<div class="setup-check-panel-head">' +
      '<div class="setup-check-panel-title">' +
        '<span class="setup-check-panel-icon">' + setupIconSvg(panelState === 'ok' ? 'plug' : 'server') + '</span>' +
        '<div><strong>' + escapeHtml(title) + '</strong><small>' + escapeHtml(subtitle || '') + '</small></div>' +
      '</div>' +
      '<span class="setup-check-summary">' + escapeHtml(summary) + '</span>' +
    '</div>' +
    '<div class="setup-check-panel-body">' + groups.map(renderCheckGroup).join('') + '</div>' +
  '</section>';
}

function renderCheckGroup(group) {
  return '<div class="setup-check-group">' +
    '<div class="setup-check-group-head">' +
      '<span class="setup-check-group-icon icon-' + checkKeyClass(group.icon) + '">' + setupIconSvg(group.icon) + '</span>' +
      '<div><strong>' + escapeHtml(group.title || '-') + '</strong><small>' + escapeHtml(group.subtitle || '') + '</small></div>' +
    '</div>' +
    '<div class="setup-check-list">' + group.items.map(renderCheckRow).join('') + '</div>' +
  '</div>';
}

function renderCheckRow(item) {
  var cls = item.pending ? 'pending' : item.ok ? 'ok' : 'fail';
  var icon = checkIconName(item);
  return '<div class="setup-check-row ' + cls + ' check-' + checkKeyClass(item.key) + ' icon-' + checkKeyClass(icon) + '">' +
    '<span class="setup-check-mark"></span>' +
    '<div class="setup-check-label"><span class="setup-check-logo">' + setupIconSvg(icon) + '</span>' + escapeHtml(item.label || item.key || '-') + '</div>' +
    '<div class="setup-check-message">' + escapeHtml(item.message || '') + '</div>' +
  '</div>';
}

function databaseIconFor(items) {
  var typeItem = (items || []).find(function(item) { return item.key === 'database_type'; });
  return typeItem && typeItem.icon ? typeItem.icon : 'database';
}

function setupIconSvg(name) {
  var icons = {
    server: '<svg viewBox="0 0 24 24"><rect x="4" y="5" width="16" height="6" rx="2"></rect><rect x="4" y="13" width="16" height="6" rx="2"></rect><path d="M8 8h.01M8 16h.01M12 8h4M12 16h4"></path></svg>',
    cpu: '<svg viewBox="0 0 24 24"><rect x="7" y="7" width="10" height="10" rx="2"></rect><path d="M4 9h3M4 15h3M17 9h3M17 15h3M9 4v3M15 4v3M9 17v3M15 17v3"></path></svg>',
    port: '<svg viewBox="0 0 24 24"><path d="M8 12h8"></path><path d="M12 8v8"></path><rect x="4" y="4" width="16" height="16" rx="4"></rect></svg>',
    database: '<svg viewBox="0 0 24 24"><ellipse cx="12" cy="6" rx="7" ry="3"></ellipse><path d="M5 6v6c0 1.7 3.1 3 7 3s7-1.3 7-3V6"></path><path d="M5 12v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6"></path></svg>',
    sqlite: '<svg viewBox="0 0 24 24"><path d="M5 17c4-7 9-10 14-12-1 5-4 10-9 15"></path><path d="M6 18c4 1 8 0 12-3"></path><path d="M5 17l-2 3 4-1"></path></svg>',
    mysql: '<svg viewBox="0 0 24 24"><path d="M4 14c2.5-5 6.3-7 11.5-6.5 2.2.2 3.8 1.2 4.5 2.5-1.7-.5-3.3-.3-4.8.6"></path><path d="M5 15c3.4 3.6 8.2 4.4 14 2"></path><path d="M15 10c1.7.8 2.8 2.2 3.2 4.2"></path></svg>',
    postgres: '<svg viewBox="0 0 24 24"><path d="M7 18c-2-1.6-3-4-2.7-7 .5-4.8 4.8-7.4 9.4-6.2 3 .8 5.3 3.3 5.8 6.4.5 3-1 5.8-3.6 7.2"></path><path d="M10 19c.2-4.8 1-8.3 2.2-10.4"></path><path d="M14 19c-.1-3.2-.6-6-1.8-8.5"></path><path d="M9 10h.01M15 10h.01"></path></svg>',
    plug: '<svg viewBox="0 0 24 24"><path d="M8 7v5M16 7v5"></path><path d="M6 12h12v2a6 6 0 0 1-12 0z"></path><path d="M12 20v-3"></path></svg>'
  };
  return icons[name] || icons.database;
}

async function doSetup() {
  if (!validateAdminForm(true)) {
    switchSetupStep('admin');
    return;
  }
  if (!setupEnvReady) {
    setSetupEnvError('本地环境检测通过后才能初始化管理员');
    return;
  }

  var btn = document.getElementById('setupBtn');
  var form = getAdminForm();

  setupSubmitting = true;
  updateSetupButton();
  if (btn) btn.textContent = '创建中...';
  setSetupEnvError('');

  try {
    var response = await fetch('/admin/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: form.username, password: form.password })
    });
    var result = await response.json();

    if (result.code === 0 && result.data && result.data.token) {
      localStorage.setItem('adminToken', result.data.token);
      location.replace('/admin');
      return;
    }
    if (result.data && Array.isArray(result.data.checks)) {
      renderSetupChecks(result.data.checks, false);
    }
    setSetupEnvError(result.message || '创建失败');
  } catch (e) {
    setSetupEnvError('网络错误，请重试');
  }

  setupSubmitting = false;
  updateSetupButton();
  if (btn) btn.textContent = '创建并进入后台';
}

(async function initSetupPage() {
  var status = await setupStatus();
  if (status && status.code === 0 && status.data && status.data.initialized) {
    location.replace('/admin');
    return;
  }

  var usernameInput = document.getElementById('setupUsernameInput');
  var passwordInput = document.getElementById('setupPasswordInput');
  var confirmInput = document.getElementById('setupPasswordConfirmInput');
  var nextBtn = document.getElementById('nextSetupBtn');
  var backBtn = document.getElementById('backSetupBtn');
  var setupBtn = document.getElementById('setupBtn');
  var checkBtn = document.getElementById('setupCheckBtn');

  switchSetupStep('admin');
  if (usernameInput) usernameInput.focus();
  if (nextBtn) nextBtn.addEventListener('click', goEnvStep);
  if (backBtn) backBtn.addEventListener('click', goAdminStep);
  if (setupBtn) setupBtn.addEventListener('click', doSetup);
  if (checkBtn) checkBtn.addEventListener('click', refreshSetupChecks);

  if (usernameInput) {
    usernameInput.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') passwordInput && passwordInput.focus();
    });
  }
  if (passwordInput) {
    passwordInput.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') confirmInput && confirmInput.focus();
    });
  }
  if (confirmInput) {
    confirmInput.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') goEnvStep();
    });
  }
})();
