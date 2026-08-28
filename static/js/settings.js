async function loadSettings() {
  var result = document.getElementById('settingsResult');
  if (result) result.innerHTML = '';
  var r = await api('GET', '/admin/settings');
  if (r.code !== 0 || !r.data) {
    showToast('配置加载失败：' + (r.message || '未知错误'), 'error');
    return;
  }
  var d = r.data;
  document.getElementById('settingMaxUpstreamCheckConcurrency').value = Number.isFinite(Number(d.maxUpstreamCheckConcurrency)) ? Number(d.maxUpstreamCheckConcurrency) : 6;
  document.getElementById('settingDispatchHealthCheckEnabled').checked = !!d.dispatchHealthCheckEnabled;
  document.getElementById('settingHealthScanEnabled').checked = d.healthScanEnabled !== false;
  document.getElementById('settingHealthScanIntervalMinutes').value = Number.isFinite(Number(d.healthScanIntervalMinutes)) ? Number(d.healthScanIntervalMinutes) : 30;
  document.getElementById('settingHealthScanBatchSize').value = Number.isFinite(Number(d.healthScanBatchSize)) ? Number(d.healthScanBatchSize) : 1000;
  document.getElementById('settingHealthScanQuietStartHour').value = Number.isFinite(Number(d.healthScanQuietStartHour)) ? Number(d.healthScanQuietStartHour) : 0;
  document.getElementById('settingHealthScanQuietEndHour').value = Number.isFinite(Number(d.healthScanQuietEndHour)) ? Number(d.healthScanQuietEndHour) : 8;
  renderHealthScanStatus(d.healthScanStatus);
  document.getElementById('settingRequestTimeoutSeconds').value = Number.isFinite(Number(d.requestTimeoutSeconds)) ? Number(d.requestTimeoutSeconds) : 45;
  document.getElementById('settingOpenApiEnabled').checked = !!d.openApiEnabled;
  var keyState = document.getElementById('openApiKeyState');
  if (keyState) {
    keyState.textContent = d.openApiKeyConfigured
      ? 'Key 已配置（明文不回显，如遗失请生成新 Key）'
      : '尚未配置，启用对接接口前需先生成';
  }
  document.getElementById('settingRateLimitEnabled').checked = !!d.rateLimitEnabled;
  document.getElementById('settingRateLimitPerMin').value = d.rateLimitPerMin || 30;
  document.getElementById('settingLoginFailLimit').value = d.loginFailLimit || 5;
  document.getElementById('settingLoginLockMinutes').value = d.loginLockMinutes || 15;
  document.getElementById('settingCaptchaEnabled').checked = !!d.captchaEnabled;
  document.getElementById('settingCaptchaSiteKey').value = d.captchaSiteKey || '';
  document.getElementById('settingCaptchaSecretKey').value = '';
  document.getElementById('settingCaptchaFreeCount').value = Number.isFinite(Number(d.captchaFreeCount)) ? Number(d.captchaFreeCount) : 0;
  document.getElementById('settingMinResponseMs').value = Number.isFinite(Number(d.minResponseMs)) ? Number(d.minResponseMs) : 150;
  document.getElementById('settingLogFileEnabled').checked = d.logFileEnabled !== false;
  document.getElementById('settingLogFilePath').value = d.logFilePath || 'logs/app.log';
  document.getElementById('settingLogMaxSizeMB').value = Number.isFinite(Number(d.logMaxSizeMB)) ? Number(d.logMaxSizeMB) : 20;
  document.getElementById('settingLogMaxBackups').value = Number.isFinite(Number(d.logMaxBackups)) ? Number(d.logMaxBackups) : 7;
  document.getElementById('settingLogMaxAgeDays').value = Number.isFinite(Number(d.logMaxAgeDays)) ? Number(d.logMaxAgeDays) : 30;
  document.getElementById('settingLogCompress').checked = !!d.logCompress;
  document.getElementById('settingAutoUpdateEnabled').checked = !!d.autoUpdateEnabled;
  var state = document.getElementById('settingCaptchaSecretState');
  if (state) state.textContent = d.captchaSecretConfigured ? 'Secret Key 已配置，留空保存不会覆盖。' : 'Secret Key 尚未配置。';
  if (typeof updateConditionalSettingsFields === 'function') updateConditionalSettingsFields();
}

function readIntSetting(id, fallback) {
  var n = parseInt(document.getElementById(id).value, 10);
  return Number.isFinite(n) ? n : fallback;
}

function fmtScanTime(v) {
  if (!v) return '从未';
  var t = new Date(v);
  if (isNaN(t.getTime())) return '从未';
  return t.toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function renderHealthScanStatus(st) {
  var el = document.getElementById('healthScanStatusText');
  if (!el) return;
  if (!st) {
    el.textContent = '暂无巡检记录';
    return;
  }

  var lines = [];
  if (st.inQuietHours) {
    lines.push('静默时段中（' + (st.quietWindow || '') + '），' +
      (st.quietResumeAt ? fmtScanTime(st.quietResumeAt) + ' 恢复' : '暂不巡检'));
  } else {
    lines.push(st.running ? '正在巡检中...' : '空闲');
  }
  lines.push('上次开始：' + fmtScanTime(st.lastStartedAt));
  if (st.lastEndedAt) {
    var detail = '检查 ' + (st.lastChecked || 0) + ' 个，状态变更 ' + (st.lastFlipped || 0) + ' 个';
    if (st.lastFailed) detail += '，未完成 ' + st.lastFailed + ' 个';
    lines.push('上次结束：' + fmtScanTime(st.lastEndedAt) + '（' + detail + '）');
  }

  var pending = Number(st.pendingTotal) || 0;
  if (pending > 0) {
    var batch = readIntSetting('settingHealthScanBatchSize', 1000);
    var rounds = Math.ceil(pending / Math.max(1, batch));
    lines.push('待刷新：' + pending + ' 个，按当前每轮数量约需 ' + rounds + ' 轮覆盖一遍');
  } else {
    lines.push('待刷新：0 个，号池状态已是最新');
  }

  if (st.lastError) lines.push('上次错误：' + st.lastError);
  el.innerHTML = lines.map(function(s) { return escapeHtml(s); }).join('<br>');
}

async function generateOpenApiKey() {
  if (!confirm('生成新 Key 会立即使旧 Key 失效，已接入的程序需要同步更新。确认继续？')) return;

  var r = await api('POST', '/admin/settings/openapi-key');
  if (r.code !== 0 || !r.data || !r.data.apiKey) {
    showToast(r.message || '生成失败', 'error');
    return;
  }

  // 明文只在这一次响应里出现，展示出来供复制
  var reveal = document.getElementById('openApiKeyReveal');
  var value = document.getElementById('openApiKeyValue');
  if (value) value.textContent = r.data.apiKey;
  if (reveal) reveal.hidden = false;

  var keyState = document.getElementById('openApiKeyState');
  if (keyState) keyState.textContent = 'Key 已配置（明文不回显，如遗失请生成新 Key）';

  showToast('已生成新 Key，请立即复制保存', 'success');
}

async function triggerHealthScan() {
  var r = await api('POST', '/admin/accounts/health-scan');
  if (r.code === 0) {
    showToast(r.message || '巡检已启动', 'success');
    // 稍等一下再刷新状态，让后台先把 running 标记写上
    setTimeout(loadSettings, 1200);
  } else {
    showToast(r.message || '巡检启动失败', 'error');
  }
}

async function saveSettings() {
  var body = {
    maxUpstreamCheckConcurrency: readIntSetting('settingMaxUpstreamCheckConcurrency', 6),
    dispatchHealthCheckEnabled: document.getElementById('settingDispatchHealthCheckEnabled').checked,
    healthScanEnabled: document.getElementById('settingHealthScanEnabled').checked,
    healthScanIntervalMinutes: readIntSetting('settingHealthScanIntervalMinutes', 30),
    healthScanBatchSize: readIntSetting('settingHealthScanBatchSize', 1000),
    healthScanQuietStartHour: readIntSetting('settingHealthScanQuietStartHour', 0),
    healthScanQuietEndHour: readIntSetting('settingHealthScanQuietEndHour', 8),
    openApiEnabled: document.getElementById('settingOpenApiEnabled').checked,
    requestTimeoutSeconds: readIntSetting('settingRequestTimeoutSeconds', 45),
    rateLimitEnabled: document.getElementById('settingRateLimitEnabled').checked,
    rateLimitPerMin: readIntSetting('settingRateLimitPerMin', 30),
    loginFailLimit: readIntSetting('settingLoginFailLimit', 5),
    loginLockMinutes: readIntSetting('settingLoginLockMinutes', 15),
    captchaEnabled: document.getElementById('settingCaptchaEnabled').checked,
    captchaSiteKey: document.getElementById('settingCaptchaSiteKey').value.trim(),
    captchaSecretKey: document.getElementById('settingCaptchaSecretKey').value.trim(),
    captchaFreeCount: readIntSetting('settingCaptchaFreeCount', 0),
    minResponseMs: readIntSetting('settingMinResponseMs', 150),
    logFileEnabled: document.getElementById('settingLogFileEnabled').checked,
    logFilePath: document.getElementById('settingLogFilePath').value.trim(),
    logMaxSizeMB: readIntSetting('settingLogMaxSizeMB', 20),
    logMaxBackups: readIntSetting('settingLogMaxBackups', 7),
    logMaxAgeDays: readIntSetting('settingLogMaxAgeDays', 30),
    logCompress: document.getElementById('settingLogCompress').checked,
    autoUpdateEnabled: document.getElementById('settingAutoUpdateEnabled').checked
  };
  var result = document.getElementById('settingsResult');
  if (result) result.innerHTML = '<div style="color:var(--text-muted);font-size:13px">正在保存...</div>';
  var r = await api('POST', '/admin/settings', body);
  if (r.code === 0) {
    showToast('配置已保存', 'success');
    await loadSettings();
    if (result) result.innerHTML = '<div class="status success">配置已保存并立即生效</div>';
  } else {
    var msg = r.message || '保存失败';
    showToast(msg, 'error');
    if (result) result.innerHTML = '<div class="status error">' + escapeHtml(msg) + '</div>';
  }
}

var currentSettingsCategory = 'base';
var settingsCategoryLabels = {
  base: '基础与安全', logging: '日志与更新', commerce: '商城与支付', delivery: '存储与通知'
};

function settingsControlBlock(id) {
  var control = document.getElementById(id);
  return control ? control.closest('.settings-field, .settings-toggle-card') : null;
}

function setSettingsBlockVisible(block, visible, displayValue) {
  if (!block) return;
  block.hidden = !visible;
  block.style.display = visible ? (displayValue || '') : 'none';
}

function createSettingsPane(host, name, controlIds) {
  var pane = document.createElement('div');
  pane.dataset.settingsPane = name;
  pane.hidden = true;
  pane.appendChild(createSettingsGridFromControls(controlIds));
  host.appendChild(pane);
  attachSettingsPanels(pane, name);
  return pane;
}

// 把高度不定的区块挂到对应分类下。
// 它们不能进 settings-grid：那里的行高被固定为 82px，内容一超就会溢出压住下面的字段。
function attachSettingsPanels(pane, category) {
  var source = document.getElementById('settingsPanelSource');
  if (!source || !pane) return;
  var panels = source.querySelectorAll('[data-settings-panel="' + category + '"]');
  for (var i = 0; i < panels.length; i++) {
    pane.appendChild(panels[i]);
  }
}

function createSettingsGridFromControls(controlIds) {
  var grid = document.createElement('div');
  grid.className = 'settings-grid';
  var columns = [0, 1, 2].map(function() { var column=document.createElement('div'); column.className='settings-column'; grid.appendChild(column); return column; });
  controlIds.forEach(function(id, index) { var block=settingsControlBlock(id); if(block) columns[index % columns.length].appendChild(block); });
  return grid;
}

function updateConditionalSettingsFields() {
  var rules = [
    ['settingRateLimitEnabled', ['settingRateLimitPerMin']],
    ['settingCaptchaEnabled', ['settingCaptchaSiteKey', 'settingCaptchaSecretKey', 'settingCaptchaFreeCount']],
    ['settingLogFileEnabled', ['settingLogFilePath', 'settingLogMaxSizeMB', 'settingLogMaxBackups', 'settingLogMaxAgeDays', 'settingLogCompress']]
  ];
  rules.forEach(function(rule) { var toggle=document.getElementById(rule[0]); if(!toggle)return; rule[1].forEach(function(id){setSettingsBlockVisible(settingsControlBlock(id),toggle.checked);}); });
  var storage=document.getElementById('setStorageType');
  if(storage){ ['setLocalPath'].forEach(function(id){setSettingsBlockVisible(settingsControlBlock(id),storage.value==='local');}); ['setS3Endpoint','setS3Region','setS3Bucket','setS3Access','setS3Secret','setS3SSL'].forEach(function(id){setSettingsBlockVisible(settingsControlBlock(id),storage.value==='s3');}); }
  var emailToggle=document.getElementById('setEmailComplete'),emailFields=document.getElementById('emailSettingsFields');if(emailToggle&&emailFields)setSettingsBlockVisible(emailFields,emailToggle.checked,'grid');
}

function initSettingsCategories() {
  if (document.getElementById('settingsCategoryHost')) return;
  var body=document.getElementById('unifiedSettingsBody');
  var source=document.getElementById('runtimeSettingsSource');
  var channelsMount=document.getElementById('commerceChannelsMount');
  var commerceMount=document.getElementById('commerceSettingsMount');
  if(!body||!source||!channelsMount||!commerceMount||!document.getElementById('settingsForm')) { return false; }
  var host=document.createElement('div'); host.id='settingsCategoryHost'; body.insertBefore(host,source);
  createSettingsPane(host,'base',['settingMaxUpstreamCheckConcurrency','settingDispatchHealthCheckEnabled','settingHealthScanEnabled','settingHealthScanIntervalMinutes','settingHealthScanBatchSize','settingHealthScanQuietStartHour','settingHealthScanQuietEndHour','settingOpenApiEnabled','settingRequestTimeoutSeconds','settingMinResponseMs','settingRateLimitEnabled','settingRateLimitPerMin','settingLoginFailLimit','settingLoginLockMinutes','settingCaptchaEnabled','settingCaptchaSiteKey','settingCaptchaSecretKey','settingCaptchaFreeCount']);
  createSettingsPane(host,'logging',['settingLogFileEnabled','settingLogFilePath','settingLogMaxSizeMB','settingLogMaxBackups','settingLogMaxAgeDays','settingLogCompress','settingAutoUpdateEnabled']);
  var commercePane=createSettingsPane(host,'commerce',['setEnabled','setDefaultExpiry','setManualExpiry']); commercePane.appendChild(channelsMount);
  var deliveryPane=createSettingsPane(host,'delivery',['setStorageType','setLocalPath','setMaxProof','setS3Endpoint','setS3Region','setS3Bucket','setS3Access','setS3Secret','setS3SSL']);
  deliveryPane.appendChild(createSettingsGridFromControls(['setEmailComplete']));
  var emailFields=createSettingsGridFromControls(['setSMTPHost','setSMTPPort','setSMTPUser','setSMTPPass','setSMTPFrom','setSMTPTLS','setEmailCard']);emailFields.id='emailSettingsFields';deliveryPane.appendChild(emailFields);
  source.remove();
  // 面板已分配到各分类，容器本身不再需要
  var panelSource=document.getElementById('settingsPanelSource'); if(panelSource)panelSource.remove();
  var settingsForm=document.getElementById('settingsForm'); if(settingsForm)settingsForm.remove();
  if(commerceMount.isConnected)commerceMount.remove();
  ['settingRateLimitEnabled','settingCaptchaEnabled','settingLogFileEnabled','setEmailComplete','setStorageType'].forEach(function(id){var el=document.getElementById(id);if(el)el.addEventListener('change',updateConditionalSettingsFields);});
  updateConditionalSettingsFields();
  var savedCategory=localStorage.getItem('settingsCategory'); if(savedCategory==='runtime'||savedCategory==='security'||savedCategory==='captcha')savedCategory='base'; if(savedCategory==='channels')savedCategory='commerce';
  switchSettingsCategory(savedCategory || 'base');
  document.querySelectorAll('[data-settings-category]').forEach(function(item) { item.addEventListener('click', function() { switchSettingsCategory(this.dataset.settingsCategory); }); });
  return true;
}

function switchSettingsCategory(name) {
  if(!settingsCategoryLabels[name])name='base';
  currentSettingsCategory=name;
  document.querySelectorAll('[data-settings-pane]').forEach(function(pane){var active=pane.dataset.settingsPane===name;pane.hidden=!active;pane.style.display=active?'block':'none';});
  document.querySelectorAll('[data-settings-category]').forEach(function(item){var active=item.dataset.settingsCategory===name;item.style.color=active?'var(--text-main)':'var(--text-muted)';item.style.fontWeight=active?'600':'400';if(active)item.setAttribute('aria-current','page');else item.removeAttribute('aria-current');});
  localStorage.setItem('settingsCategory',name);
  updateConditionalSettingsFields();
}

async function saveCurrentSettingsCategory() {
  if(currentSettingsCategory==='commerce'||currentSettingsCategory==='delivery')await saveCommerceSettingsConfig();
  else await saveSettings();
}
