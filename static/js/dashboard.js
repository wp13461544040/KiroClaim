// 概览页面模块

// 自动刷新定时器
let dashboardRefreshTimer = null;
const DASHBOARD_REFRESH_INTERVAL = 30000; // 30秒
let recentActivityPage = 1;
const RECENT_ACTIVITY_PAGE_SIZE = 10;

// 启动自动刷新
function startDashboardAutoRefresh() {
  stopDashboardAutoRefresh();
  dashboardRefreshTimer = setInterval(() => {
    loadStats();
  }, DASHBOARD_REFRESH_INTERVAL);
}

// 停止自动刷新
function stopDashboardAutoRefresh() {
  if (dashboardRefreshTimer) {
    clearInterval(dashboardRefreshTimer);
    dashboardRefreshTimer = null;
  }
}

async function loadStats() {
  // 动态骨架屏加载指示器
  const statIds = [
    'statAccTotal','statAccUnused','statCardUnused',
    'statTodayActivate'
  ];
  statIds.forEach(function(id) {
    var el = document.getElementById(id);
    if (el) {
      el.textContent = '\u00a0';
      el.classList.add('k-skeleton');
    }
  });

  // 图表骨架屏
  showChartSkeleton('accountStatusChart', 4);
  showChartSkeleton('cardDistributionChart', 5);

  const r = await api('GET', '/admin/pool/stats');

  // 移除骨架屏
  statIds.forEach(function(id) {
    var el = document.getElementById(id);
    if (el) el.classList.remove('k-skeleton');
  });

  if (r.code !== 0) {
    statIds.forEach(function(id) {
      var el = document.getElementById(id);
      if (el) el.textContent = '-';
    });
    return;
  }

  // 更新核心指标
  var accounts = r.data.accounts || {};
  var cards = r.data.cards || {};
  document.getElementById('statAccTotal').textContent = accounts.total || 0;
  // 注意不能用 `available || unused`：可用账号为 0 时会被当成假值，
  // 错误地回退成账号池总数，导致概览显示的「可用账号」比实际能发货的多。
  document.getElementById('statAccUnused').textContent =
    accounts.available != null ? accounts.available : (accounts.unused || 0);
  document.getElementById('statCardUnused').textContent = cards.unused || 0;

  // 今日统计
  var today = r.data.today || {};
  document.getElementById('statTodayActivate').textContent = today.activate || 0;

  // 账号状态统计：账号池 / 已分配分栏，口径与两个列表的健康状态筛选一致
  const statusGroups = accounts.statusGroups || {};
  drawAccountStatusChart(statusGroups);

  // 卡密分布统计
  const cardDist = cards.distribution || [];
  drawCardDistributionChart(cardDist);

  // 加载最近活动
  loadRecentActivity();
}

function emptyChart() {
  return '<div class="cute-empty-chart">暂无数据</div>';
}

// 账号状态图表：账号池（未分配）与已分配分栏展示。
//
// 数字与两个列表「健康状态」筛选的结果一一对应：
//   - 正常：未封禁且没用过额度
//   - 额度已用：未封禁且用过额度（credit_used > 0），封禁优先
//   - 已封禁：status = suspended
// 三项之和恒等于该栏账号总数，避免概览与列表数量打架。
//
// 健康率 = 非封禁账号 / 总账号，即（正常 + 额度已用）/ 总数。
// 额度已用的账号只是不能发货，仍然不是封禁号，所以必须计入健康率。
function drawAccountStatusChart(statusGroups) {
  const canvas = document.getElementById('accountStatusChart');
  if (!canvas) return;

  const groups = statusGroups || {};
  const pool = normalizeStatusGroup(groups.pool);
  const assigned = normalizeStatusGroup(groups.assigned);

  const grandTotal = pool.total + assigned.total;
  if (grandTotal === 0) {
    canvas.innerHTML = emptyChart();
    return;
  }

  // 全部账号的健康率：非封禁 / 总账号
  const nonSuspended = grandTotal - pool.suspended - assigned.suspended;

  canvas.innerHTML =
    '<div class="k-status-summary" title="健康率 = 非封禁账号 / 总账号">' +
      '<span>全部账号 <strong>' + grandTotal + '</strong></span>' +
      '<span class="k-status-summary-sep">·</span>' +
      '<span>非封禁 <strong>' + nonSuspended + '</strong></span>' +
      '<span class="k-status-summary-sep">·</span>' +
      '<span>健康率 <strong>' + formatStatusShare(nonSuspended, grandTotal) + '</strong></span>' +
    '</div>' +
    '<div class="k-status-groups">' +
      renderStatusGroup('账号池', '未分配', pool) +
      renderStatusGroup('已分配', '已发给买家', assigned) +
    '</div>';
}

// 后端字段缺失时用三项之和兜底，保证「共 N 个」不会显示成 0。
function normalizeStatusGroup(group) {
  const active = Math.max(0, Number(group && group.active) || 0);
  const used = Math.max(0, Number(group && group.used) || 0);
  const suspended = Math.max(0, Number(group && group.suspended) || 0);
  const total = Math.max(0, Number(group && group.total) || (active + used + suspended));
  return { active: active, used: used, suspended: suspended, total: total };
}

function formatStatusShare(count, total) {
  if (total <= 0) return '0%';
  const pct = count / total * 100;
  if (count > 0 && pct < 0.1) return '&lt;0.1%';
  if (pct >= 99.95) return '100%';
  return (Math.round(pct * 10) / 10) + '%';
}

function renderStatusGroup(title, subtitle, group) {
  const rows = [
    { key: 'active', label: '正常', cls: 'mint' },
    { key: 'used', label: '额度已用', cls: 'amber' },
    { key: 'suspended', label: '已封禁', cls: 'rose' }
  ];

  const listHtml = rows.map(function(row) {
    const count = group[row.key] || 0;
    return '<div class="cute-stat-item ' + row.cls + '">' +
      '<div class="cute-stat-info">' +
        '<span class="cute-dot"></span>' +
        '<span class="cute-stat-label">' + row.label + '</span>' +
      '</div>' +
      '<div class="k-status-right">' +
        '<span class="cute-stat-value">' + count + '</span>' +
        '<span class="k-status-share">' + formatStatusShare(count, group.total) + '</span>' +
      '</div>' +
    '</div>';
  }).join('');

  // 健康率 = 非封禁 / 该栏总数；额度已用不算封禁，仍计入健康率
  const healthy = group.total - group.suspended;
  const healthText = group.total > 0
    ? '共 ' + group.total + ' 个 · 健康率 ' + formatStatusShare(healthy, group.total)
    : '共 0 个';

  return '<section class="k-status-group">' +
    '<div class="k-status-group-head">' +
      '<div class="k-status-group-title">' +
        '<span class="k-status-group-name">' + title + '</span>' +
        '<span class="k-status-group-sub">' + subtitle + '</span>' +
      '</div>' +
      '<span class="k-status-group-total" title="健康率 = 非封禁账号 / 该栏账号总数">' + healthText + '</span>' +
    '</div>' +
    '<div class="cute-status-list">' + listHtml + '</div>' +
  '</section>';
}

// 绘制卡密分布图表（表格模式，每个卡密类型为一列）
function drawCardDistributionChart(distribution) {
  const canvas = document.getElementById('cardDistributionChart');
  if (!canvas) return;

  if (!distribution || distribution.length === 0) {
    canvas.innerHTML = emptyChart();
    return;
  }

  // 按 total 降序排序，取前 10 个
  const sortedDist = [...distribution].sort((a, b) => (b.total || 0) - (a.total || 0)).slice(0, 10);

  // 构建表头：每个卡密类型为一列
  const theadHtml = `
    <thead>
      <tr>
        <th class="k-dist-th-corner">类型</th>
        ${sortedDist.map(d => `<th class="k-dist-th-card">${d.label || '未知'}</th>`).join('')}
      </tr>
    </thead>
  `;

  // 构建表体：三行（未使用、已使用、总计）
  const tbodyHtml = `
    <tbody>
      <tr>
        <th class="k-dist-th-row">未使用</th>
        ${sortedDist.map(d => `<td class="k-dist-td-unused">${d.unused || 0}</td>`).join('')}
      </tr>
      <tr>
        <th class="k-dist-th-row">已使用</th>
        ${sortedDist.map(d => `<td class="k-dist-td-used">${d.used || 0}</td>`).join('')}
      </tr>
      <tr>
        <th class="k-dist-th-row">总计</th>
        ${sortedDist.map(d => `<td class="k-dist-td-total">${d.total || 0}</td>`).join('')}
      </tr>
    </tbody>
  `;

  canvas.innerHTML = `
    <div class="k-card-dist-table-wrapper">
      <table class="k-card-dist-table">
        ${theadHtml}
        ${tbodyHtml}
      </table>
    </div>
  `;
}

// 显示图表骨架屏
function showChartSkeleton(elementId, barCount) {
  const el = document.getElementById(elementId);
  if (!el) return;
  const heights = [];
  for (var i = 0; i < barCount; i++) {
    heights.push(30 + Math.round(Math.random() * 60));
  }
  el.innerHTML = '<div class="k-chart-skeleton">' +
    heights.map(function(h) { return '<div class="k-bar" style="height:' + h + '%"></div>'; }).join('') +
    '</div>';
}

// 最近活动
var recentActionLabels = {
  activate: '激活',
  import: '导入',
  delete: '删除',
  generate: '生成卡密',
  clear: '清空',
  refresh: '刷新',
  logout: '登出',
  settings: '设置',
  export: '导出'
};

async function loadRecentActivity(page) {
  var tbody = document.getElementById('recentActivityBody');
  if (!tbody) return;
  if (!page) page = recentActivityPage;
  recentActivityPage = page;

  var r = await api('GET', '/admin/oplogs?page=' + page + '&size=' + RECENT_ACTIVITY_PAGE_SIZE);
  if (r.code !== 0 || !r.data.list || !r.data.list.length) {
    tbody.innerHTML = '<tr><td colspan="3" style="text-align:center;color:#999;padding:24px;font-size:13px">暂无活动记录</td></tr>';
    var emptyPagination = document.getElementById('recentActivityPagination');
    if (emptyPagination) emptyPagination.innerHTML = '';
    return;
  }

  tbody.innerHTML = r.data.list.map(function(log) {
    var label = recentActionLabels[log.Action] || log.Action;
    var timeStr = new Date(log.CreatedAt).toLocaleString('zh-CN', {hour12: false});
    return '<tr>' +
      '<td class="recent-action" data-label="操作" style="font-size:13px;white-space:nowrap">' + label + '</td>' +
      '<td class="recent-detail" data-label="详情" style="font-size:12px;color:var(--text-muted)">' + (log.Detail || '-') + '</td>' +
      '<td class="recent-time" data-label="时间" style="font-size:12px;color:#999;white-space:nowrap">' + timeStr + '</td>' +
    '</tr>';
  }).join('');

  renderPagination('recentActivityPagination', r.data.total || 0, r.data.size || RECENT_ACTIVITY_PAGE_SIZE, r.data.page || page, loadRecentActivity);
}
