// UI 工具函数模块

// 状态徽章（三态）
function healthBadge(status) {
  if (status === 'suspended') {
    return '<span style="display:inline-flex;align-items:center;gap:6px"><span style="width:8px;height:8px;background:#ef4444;border-radius:50%"></span>已封禁</span>';
  }
  if (status === 'used') {
    return '<span style="display:inline-flex;align-items:center;gap:6px"><span style="width:8px;height:8px;background:#f59e0b;border-radius:50%"></span>额度已用</span>';
  }
  // 默认：active 或任何未知值都当作正常（瞬时故障不建独立态）
  return '<span style="display:inline-flex;align-items:center;gap:6px"><span style="width:8px;height:8px;background:#10b981;border-radius:50%"></span>正常</span>';
}

// 订阅级别标签：FREE / PRO / PRO+ / POWER，输入可能是 "KIRO FREE" / "Pro" 等
function subscriptionBadge(sub) {
  if (!sub) return '<span style="color:#a1a1aa">-</span>';
  // 抽取 tier 关键字（忽略 "KIRO " 前缀、忽略大小写）
  var raw = String(sub).trim();
  var upper = raw.toUpperCase();
  var tier = upper.replace(/^KIRO\s+/, '');
  // 规范化：POWER / PRO+ / PRO / FREE；其他一律原样
  var known = {
    'FREE':  { label: 'FREE',  bg: '#f4f4f5', fg: '#52525b', bd: '#e4e4e7' },
    'PRO':   { label: 'PRO',   bg: '#eff6ff', fg: '#1d4ed8', bd: '#bfdbfe' },
    'PRO+':  { label: 'PRO+',  bg: '#f5f3ff', fg: '#6d28d9', bd: '#ddd6fe' },
    'POWER': { label: 'POWER', bg: '#fff7ed', fg: '#c2410c', bd: '#fed7aa' }
  };
  var t = known[tier];
  if (!t) {
    // 兜底：未知订阅名显示成中性 tag
    t = { label: raw, bg: '#f4f4f5', fg: '#52525b', bd: '#e4e4e7' };
  }
  return '<span class="subscription-badge" style="display:inline-flex;align-items:center;padding:2px 10px;' +
    'border-radius:999px;font-size:11px;font-weight:600;letter-spacing:0.3px;' +
    'background:' + t.bg + ';color:' + t.fg + ';border:1px solid ' + t.bd + '">' +
    t.label + '</span>';
}

// Toast 提示
function showToast(message, type) {
  if (!type) type = 'info';
  var toast = document.createElement('div');
  var style = 'position:fixed;top:24px;right:24px;padding:14px 18px;border-radius:6px;font-size:13px;font-weight:500;z-index:9999;animation:slideIn 0.3s ease;box-shadow:0 4px 12px rgba(0,0,0,0.1);';
  if (type === 'success') style += 'background:#000;color:#fff;';
  else if (type === 'error') style += 'background:#fff;color:#991b1b;border:1px solid #fecaca;';
  else style += 'background:#fff;color:#171717;border:1px solid #eaeaea;';
  toast.style.cssText = style;
  toast.textContent = message;
  document.body.appendChild(toast);
  setTimeout(function() {
    toast.style.animation = 'slideOut 0.3s ease';
    setTimeout(function() { toast.remove(); }, 300);
  }, 3000);
}

// 下拉框控制
var currentDropdown = null;

function toggleDropdown(id) {
  var dropdown = document.getElementById(id);
  if (currentDropdown && currentDropdown !== dropdown) {
    currentDropdown.classList.remove('active');
  }
  dropdown.classList.toggle('active');
  currentDropdown = dropdown.classList.contains('active') ? dropdown : null;
}

document.addEventListener('click', function(e) {
  if (!e.target.closest('.k-dropdown')) {
    document.querySelectorAll('.k-dropdown').forEach(function(d) {
      d.classList.remove('active');
    });
    currentDropdown = null;
  }
});


// 每页条数偏好（按列表维度持久化）
var PAGE_SIZE_OPTIONS = [15, 20, 30, 50, 100, 200];

function getPageSize(listKey, fallback) {
  var def = fallback || 15;
  if (!listKey) return def;
  try {
    var v = parseInt(localStorage.getItem('pageSize.' + listKey), 10);
    return PAGE_SIZE_OPTIONS.indexOf(v) >= 0 ? v : def;
  } catch (e) {
    return def;
  }
}

function setPageSize(listKey, size) {
  if (!listKey) return;
  try {
    localStorage.setItem('pageSize.' + listKey, String(size));
  } catch (e) { /* 隐私模式下忽略 */ }
}

// 计算页码按钮序列，超长时用省略号收敛，避免总页数很大时渲染上千个按钮
function __pageWindow(current, pages) {
  if (pages <= 7) {
    var all = [];
    for (var i = 1; i <= pages; i++) all.push(i);
    return all;
  }
  var out = [1];
  var from = Math.max(2, current - 1);
  var to = Math.min(pages - 1, current + 1);
  if (from > 2) out.push('...');
  for (var p = from; p <= to; p++) out.push(p);
  if (to < pages - 1) out.push('...');
  out.push(pages);
  return out;
}

// 分页渲染
// listKey 可选：传入后显示"每页条数"选择器，并把选择持久化
function renderPagination(containerId, total, size, current, fn, listKey) {
  var el = document.getElementById(containerId);
  if (!el) return;
  total = Number(total) || 0;
  size = Number(size) || 15;
  current = Number(current) || 1;
  var pages = Math.max(1, Math.ceil(total / size));
  if (current > pages) current = pages;

  // 单页且没有条数选择器时不占位
  if (pages <= 1 && !listKey) { el.innerHTML = ''; return; }

  var jumpId = containerId + '_jump';
  var sizeId = containerId + '_size';
  var fnName = fn.name;
  var from = total === 0 ? 0 : (current - 1) * size + 1;
  var to = Math.min(total, current * size);

  var html = '<div class="k-pager">';

  html += '<span class="k-pager-total">共 ' + total + ' 条';
  if (total > 0) html += '，当前 ' + from + '-' + to;
  html += '</span>';

  if (pages > 1) {
    html += '<div class="k-pager-nav">';
    html += '<button class="ui-btn ui-btn-secondary ui-btn-sm"' + (current <= 1 ? ' disabled' : '') +
      ' onclick="' + fnName + '(' + (current - 1) + ')">上一页</button>';

    var seq = __pageWindow(current, pages);
    for (var i = 0; i < seq.length; i++) {
      if (seq[i] === '...') {
        html += '<span class="k-pager-gap">...</span>';
      } else if (seq[i] === current) {
        html += '<button class="ui-btn ui-btn-sm k-pager-active" aria-current="page">' + seq[i] + '</button>';
      } else {
        html += '<button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="' + fnName + '(' + seq[i] + ')">' + seq[i] + '</button>';
      }
    }

    html += '<button class="ui-btn ui-btn-secondary ui-btn-sm"' + (current >= pages ? ' disabled' : '') +
      ' onclick="' + fnName + '(' + (current + 1) + ')">下一页</button>';
    html += '</div>';
  }

  if (listKey) {
    html += '<span class="k-pager-size">每页' +
      '<select id="' + sizeId + '" class="k-pager-select" onchange="__pageSizeChange(\'' + listKey + '\',this.value,' + fnName + ')">';
    for (var s = 0; s < PAGE_SIZE_OPTIONS.length; s++) {
      var opt = PAGE_SIZE_OPTIONS[s];
      html += '<option value="' + opt + '"' + (opt === size ? ' selected' : '') + '>' + opt + '</option>';
    }
    html += '</select>条</span>';
  }

  if (pages > 1) {
    html += '<span class="k-pager-jump">跳至' +
      '<input id="' + jumpId + '" type="number" min="1" max="' + pages + '" value="' + current + '" ' +
        'class="k-pager-input" ' +
        'onkeydown="if(event.key===\'Enter\'){__pageJump(\'' + jumpId + '\',' + pages + ',' + fnName + ')}">' +
      '/ ' + pages + ' 页' +
      '<button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="__pageJump(\'' + jumpId + '\',' + pages + ',' + fnName + ')">GO</button>' +
      '</span>';
  }

  html += '</div>';
  el.innerHTML = html;
}

// 跳转页处理
function __pageJump(inputId, maxPages, fn) {
  var input = document.getElementById(inputId);
  if (!input) return;
  var p = parseInt(input.value, 10);
  if (isNaN(p) || p < 1) p = 1;
  if (p > maxPages) p = maxPages;
  fn(p);
}

// 每页条数变更：持久化后回到第一页重新加载
function __pageSizeChange(listKey, value, fn) {
  var size = parseInt(value, 10);
  if (PAGE_SIZE_OPTIONS.indexOf(size) < 0) return;
  setPageSize(listKey, size);
  fn(1);
}

// 复制到剪贴板
function copyToClipboard(text) {
  navigator.clipboard.writeText(text).then(function() {
    showToast('已复制到剪贴板', 'success');
  }).catch(function() {
    showToast('复制失败', 'error');
  });
}


// 侧边栏切换（移动端）
function toggleSidebar() {
  var sidebar = document.getElementById('sidebar');
  var overlay = document.getElementById('sidebarOverlay');
  if (sidebar.classList.contains('open')) {
    sidebar.classList.remove('open');
    overlay.classList.remove('active');
  } else {
    sidebar.classList.add('open');
    overlay.classList.add('active');
  }
}

// 切换页面时自动关闭侧边栏（移动端）
function closeSidebarOnMobile() {
  if (window.innerWidth <= 768) {
    var sidebar = document.getElementById('sidebar');
    var overlay = document.getElementById('sidebarOverlay');
    if (sidebar) sidebar.classList.remove('open');
    if (overlay) overlay.classList.remove('active');
  }
}

function applySidebarCollapsed(collapsed) {
  var app = document.getElementById('appContainer');
  var btn = document.getElementById('sidebarCollapseToggle');
  if (!app) return;

  app.classList.toggle('sidebar-collapsed', !!collapsed);
  if (btn) {
    var label = collapsed ? '展开侧边栏' : '收起侧边栏';
    btn.setAttribute('aria-label', label);
    btn.setAttribute('aria-pressed', collapsed ? 'true' : 'false');
    btn.title = label;
  }
}

function toggleSidebarCollapse() {
  var app = document.getElementById('appContainer');
  if (!app) return;

  var collapsed = !app.classList.contains('sidebar-collapsed');
  localStorage.setItem('sidebarCollapsed', collapsed ? '1' : '0');
  applySidebarCollapsed(collapsed);
}

function initSidebarCollapse() {
  applySidebarCollapsed(localStorage.getItem('sidebarCollapsed') === '1');
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initSidebarCollapse);
} else {
  initSidebarCollapse();
}
