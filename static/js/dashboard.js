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
    'statTodayActivate',
    'statActive','statSuspended'
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
  document.getElementById('statAccTotal').textContent = r.data.accounts.total;
  document.getElementById('statAccUnused').textContent = r.data.accounts.available || r.data.accounts.unused;
  document.getElementById('statCardUnused').textContent = r.data.cards.unused;

  // 今日统计
  var today = r.data.today || {};
  document.getElementById('statTodayActivate').textContent = today.activate || 0;

  // 账号状态统计（两态）
  const statusCount = r.data.accounts.status || { active: 0, suspended: 0 };
  document.getElementById('statActive').textContent = statusCount.active;
  document.getElementById('statSuspended').textContent = statusCount.suspended;
  drawAccountStatusChart(statusCount);

  // 加载最近活动
  loadRecentActivity();
}

function emptyChart() {
  return '<div class="cute-empty-chart">暂无数据</div>';
}

function drawAccountStatusChart(statusCount) {
  const canvas = document.getElementById('accountStatusChart');
  if (!canvas) return;

  const active = Math.max(0, statusCount.active || 0);
  const suspended = Math.max(0, statusCount.suspended || 0);
  const total = active + suspended;

  if (total === 0) {
    canvas.innerHTML = emptyChart();
    return;
  }

  const pct = active / total * 100;
  const R = 64;
  const C = 2 * Math.PI * R;
  const activeLen = (active / total) * C;
  const suspendedLen = C - activeLen;
  const pctLabel = pct >= 99.95 ? '100' : pct.toFixed(pct >= 10 ? 1 : 2);

  canvas.innerHTML = `
    <div class="cute-status-chart">
      <div class="cute-stat-pill mint">
        <div class="cute-stat-label">
          <span class="cute-dot"></span>
          <span>正常</span>
        </div>
        <div class="cute-stat-number">${active}</div>
      </div>

      <div class="cute-donut">
        <svg width="160" height="160" viewBox="0 0 160 160" class="cute-donut-svg">
          <defs>
            <linearGradient id="gradActive" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stop-color="#7dd3fc"/>
              <stop offset="100%" stop-color="#34d399"/>
            </linearGradient>
            <linearGradient id="gradSuspended" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stop-color="#fda4af"/>
              <stop offset="100%" stop-color="#fb7185"/>
            </linearGradient>
          </defs>
          <circle cx="80" cy="80" r="${R}" fill="none" stroke="#f4f4f5" stroke-width="18"/>
          ${active > 0 ? `<circle cx="80" cy="80" r="${R}" fill="none"
              stroke="url(#gradActive)" stroke-width="18" stroke-linecap="round"
              stroke-dasharray="${activeLen} ${C - activeLen}"
              stroke-dashoffset="0"
              class="cute-ring-segment"/>` : ''}
          ${suspended > 0 ? `<circle cx="80" cy="80" r="${R}" fill="none"
              stroke="url(#gradSuspended)" stroke-width="18" stroke-linecap="round"
              stroke-dasharray="${suspendedLen} ${C - suspendedLen}"
              stroke-dashoffset="${-activeLen}"
              class="cute-ring-segment delayed"/>` : ''}
        </svg>
        <div class="cute-donut-center">
          <div class="cute-donut-value">
            ${pctLabel}<span>%</span>
          </div>
          <div class="cute-donut-label">健康率</div>
        </div>
      </div>

      <div class="cute-stat-pill rose">
        <div class="cute-stat-label">
          <span class="cute-dot"></span>
          <span>已封禁</span>
        </div>
        <div class="cute-stat-number">${suspended}</div>
      </div>
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
