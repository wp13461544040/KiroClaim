var VERSION_INFO = null;
var VERSION_LOADING = false;
var VERSION_PROMISE = null;

function versionEscape(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function handleVersionKeydown(event) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault();
    showVersionPanel();
  }
}

async function loadVersionBadge(force) {
  if (VERSION_LOADING && VERSION_PROMISE) return VERSION_PROMISE;
  VERSION_LOADING = true;
  VERSION_PROMISE = (async function() {
    try {
      var r = await api('GET', '/admin/version' + (force ? '?force=1' : ''));
      if (r.code === 0 && r.data) {
        VERSION_INFO = r.data;
        renderVersionBadge(r.data);
        return r.data;
      }
    } catch (e) {
      // 版本检测失败不影响后台主流程。
    } finally {
      VERSION_LOADING = false;
      VERSION_PROMISE = null;
    }
    return VERSION_INFO;
  })();
  return VERSION_PROMISE;
}

function renderVersionBadge(data) {
  var badge = document.getElementById('sidebarVersion');
  var label = document.getElementById('sidebarVersionText');
  var mark = document.getElementById('sidebarVersionNew');
  if (!badge || !label || !data) return;

  var hasUpdate = !!data.hasUpdate;
  label.textContent = data.currentVersion || 'dev';
  badge.classList.toggle('has-update', hasUpdate);
  badge.title = hasUpdate ? '发现新版本 ' + data.latestVersion : '当前版本 ' + (data.currentVersion || 'dev');
  if (mark) mark.hidden = !hasUpdate;
}

async function showVersionPanel() {
  var data = VERSION_INFO || await loadVersionBadge(false);
  if (!data) {
    showToast('版本信息读取失败', 'error');
    return;
  }
  renderVersionModal(data);
}

function renderVersionModal(data) {
  var old = document.getElementById('versionModal');
  if (old) {
    updateVersionModal(data);
    return;
  }

  var overlay = document.createElement('div');
  overlay.id = 'versionModal';
  overlay.className = 'modal-overlay active';
  overlay.innerHTML =
    '<div class="modal-content" style="max-width:560px">' +
      '<div class="modal-header">' +
        '<span class="modal-title">系统版本</span>' +
        '<button type="button" class="modal-close" aria-label="关闭" title="关闭" onclick="closeVersionModal()">&times;</button>' +
      '</div>' +
      '<div class="modal-body" id="versionModalBody">' +
        buildVersionModalBody(data) +
      '</div>' +
    '</div>';
  document.body.appendChild(overlay);
}

function updateVersionModal(data) {
  var body = document.getElementById('versionModalBody');
  if (body) body.innerHTML = buildVersionModalBody(data);
}

function buildVersionModalBody(data) {
  var latest = data.latestVersion || '-';
  var current = data.currentVersion || 'dev';
  var statusText = data.error ? ('检测失败：' + data.error) : (data.hasUpdate ? '发现可用更新' : '当前已是最新版本');
  var support = data.updateSupported
    ? '当前环境支持应用内 Docker 更新。'
    : '应用内更新需要容器可访问宿主 Docker，并挂载 compose 文件；否则请在服务器执行 docker compose pull && docker compose up -d。';

  var updateBtn = '';
  if (data.hasUpdate) {
    updateBtn = '<button class="ui-btn ui-btn-primary" id="versionUpdateBtn" onclick="triggerDockerUpdate()" ' +
      (data.updateInProgress ? 'disabled' : '') + '>更新到最新 Docker</button>';
  }

  return '' +
    '<div class="version-info-grid">' +
      '<div class="version-info-row"><span>当前版本</span><strong>' + versionEscape(current) + '</strong></div>' +
      '<div class="version-info-row"><span>最新版本</span><strong>' + versionEscape(latest) + '</strong></div>' +
      '<div class="version-info-row"><span>镜像</span><strong>' + versionEscape(data.dockerImage || '-') + '</strong></div>' +
      '<div class="version-info-row"><span>状态</span><strong>' + versionEscape(statusText) + '</strong></div>' +
    '</div>' +
    '<div class="version-update-note">' + versionEscape(support) + '</div>' +
    (data.lastUpdateMessage ? '<div class="version-update-note">' + versionEscape(data.lastUpdateMessage) + '</div>' : '') +
    '<div class="version-actions">' +
      '<button class="ui-btn ui-btn-secondary version-refresh-btn" id="versionRefreshBtn" onclick="refreshVersionPanel()">重新检测</button>' +
      (data.releaseUrl ? '<a class="ui-btn ui-btn-secondary" href="' + versionEscape(data.releaseUrl) + '" target="_blank" rel="noopener">查看 Release</a>' : '') +
      updateBtn +
    '</div>';
}

function closeVersionModal() {
  var modal = document.getElementById('versionModal');
  if (modal) modal.remove();
}

async function refreshVersionPanel() {
  var btn = document.getElementById('versionRefreshBtn');
  if (btn) {
    btn.disabled = true;
    btn.classList.add('loading');
    btn.innerHTML = '<span class="version-refresh-spinner" aria-hidden="true"></span><span>检测中</span>';
  }

  var data = await loadVersionBadge(true);
  if (data) {
    updateVersionModal(data);
  } else if (btn) {
    btn.disabled = false;
    btn.classList.remove('loading');
    btn.textContent = '重新检测';
  }
}

async function triggerDockerUpdate() {
  if (!confirm('确认更新到最新 Docker 镜像吗？更新过程会拉取镜像并重建容器。')) return;
  var btn = document.getElementById('versionUpdateBtn');
  if (btn) {
    btn.disabled = true;
    btn.textContent = '更新中...';
  }
  var r = await api('POST', '/admin/version/update', {});
  if (r.code === 0) {
    showToast(r.message || '更新任务已启动', 'success');
    await refreshVersionPanel();
  } else {
    showToast(r.message || '更新失败', 'error');
    if (btn) {
      btn.disabled = false;
      btn.textContent = '更新到最新 Docker';
    }
  }
}
