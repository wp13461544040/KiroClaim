// 操作日志模块

var logActionFilter = '';

// 操作类型中文映射
var actionLabels = {
  import: '导入',
  delete: '删除',
  generate: '生成卡密',
  activate: '激活',
  clear: '清空',
  refresh: '刷新',
  logout: '登出',
  settings: '设置',
  export: '导出'
};

// 加载日志列表
async function loadLogs(page) {
  if (!page) page = 1;
  var size = getPageSize('logs', 20);
  var url = '/admin/oplogs?page=' + page + '&size=' + size;
  if (logActionFilter) url += '&action=' + logActionFilter;

  var r = await api('GET', url);
  var tbody = document.getElementById('logsBody');
  if (r.code !== 0 || !r.data.list || !r.data.list.length) {
    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;padding:40px">暂无日志记录</td></tr>';
    renderPagination('logsPagination', 0, size, 1, loadLogs, 'logs');
    return;
  }
  tbody.innerHTML = r.data.list.map(function(log) {
    var label = actionLabels[log.Action] || log.Action;
    var ip = log.ClientIP || '-';
    var ua = log.UserAgent || '';
    var ipTooltip = ua ? ' title="' + ua.replace(/"/g, '&quot;') + '"' : '';
    return '<tr>' +
      '<td style="width:40px"><input type="checkbox" class="log-checkbox" data-id="' + log.ID + '"></td>' +
      '<td data-label="ID" style="color:#999">' + log.ID + '</td>' +
      '<td data-label="操作类型">' + label + '</td>' +
      '<td data-label="详情" style="font-size:13px;word-break:break-all">' + (log.Detail || '-') + '</td>' +
      '<td data-label="操作者">' + (log.Operator || '-') + '</td>' +
      '<td data-label="客户端 IP" style="font-size:12px;color:#999;font-family:monospace"' + ipTooltip + '>' + ip + '</td>' +
      '<td data-label="时间" style="color:#999;font-size:12px">' + new Date(log.CreatedAt).toLocaleString('zh-CN', {hour12:false}) + '</td>' +
    '</tr>';
  }).join('');
  renderPagination('logsPagination', r.data.total, size, page, loadLogs, 'logs');
  
  // 重置全选复选框
  var selectAllCheckbox = document.getElementById('logsSelectAll');
  if (selectAllCheckbox) selectAllCheckbox.checked = false;
}

// 日志类型筛选
function selectLogAction(value, text) {
  logActionFilter = value;
  document.getElementById('logActionText').textContent = text;
  document.querySelectorAll('#logActionDropdown .k-dropdown-item').forEach(function(item) {
    item.classList.remove('selected');
  });
  event.target.classList.add('selected');
  toggleDropdown('logActionDropdown');
  loadLogs(1);
}

// 全选/反选日志
function toggleSelectAllLogs(checked) {
  document.querySelectorAll('.log-checkbox').forEach(function(cb) {
    cb.checked = checked;
  });
}

// 批量删除日志
async function batchDeleteLogs() {
  var checkedBoxes = document.querySelectorAll('.log-checkbox:checked');
  if (checkedBoxes.length === 0) {
    showToast('请先选择要删除的日志', 'warning');
    return;
  }
  
  if (!confirm('确定要删除选中的 ' + checkedBoxes.length + ' 条日志吗？此操作不可恢复！')) {
    return;
  }
  
  var ids = Array.from(checkedBoxes).map(function(cb) {
    return parseInt(cb.getAttribute('data-id'));
  });
  
  var r = await api('POST', '/admin/oplogs/batch-delete', { ids: ids });
  if (r.code === 0) {
    showToast('已删除 ' + (r.data.deleted || ids.length) + ' 条日志', 'success');
    loadLogs();
  } else {
    showToast(r.message || '删除失败', 'error');
  }
}

// 清空所有日志
async function clearAllLogs() {
  if (!confirm('确定要清空所有操作日志吗？此操作不可恢复！')) {
    return;
  }
  
  if (!confirm('再次确认：这将删除所有历史日志记录，确定继续吗？')) {
    return;
  }
  
  var r = await api('POST', '/admin/oplogs/clear');
  if (r.code === 0) {
    showToast('日志已清空，共删除 ' + (r.data.deleted || 0) + ' 条记录', 'success');
    loadLogs();
  } else {
    showToast(r.message || '清空失败', 'error');
  }
}
