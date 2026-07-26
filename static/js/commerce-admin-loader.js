var commerceAdminLoaded = false;
var commerceAdminLoading = null;

function commercePageCard(title, content) {
  return '<div class="k-card"><div class="k-card-header"><span class="k-card-title">' + title + '</span><a class="ui-btn ui-btn-secondary ui-btn-sm" href="/" target="_blank" rel="noopener">购买页</a></div><div class="k-card-body">' + content + '</div></div>';
}

async function loadCommerceAdmin() {
  if (commerceAdminLoaded) return;
  if (commerceAdminLoading) return commerceAdminLoading;
  commerceAdminLoading = (async function() {
    var response = await fetch('/static/commerce-admin-fragment.html');
    if (!response.ok) throw new Error('商城管理界面加载失败');
    var doc = new DOMParser().parseFromString(await response.text(), 'text/html');
    var pages = [
      ['commerceOrdersMount', 'ordersPanel', '商城订单'],
      ['commerceChannelsMount', 'channelsPanel', '支付渠道'],
      ['commerceSettingsMount', 'settingsPanel', '商城设置']
    ];
    pages.forEach(function(page) {
      var source = doc.getElementById(page[1]);
      if (!source) throw new Error('商城管理界面结构无效');
      document.getElementById(page[0]).innerHTML = page[1] === 'ordersPanel' ? commercePageCard(page[2], source.innerHTML) : source.innerHTML;
    });
    var script = document.createElement('script');
    script.src = '/static/js/commerce-admin.js';
    await new Promise(function(resolve, reject) { script.onload = resolve; script.onerror = reject; document.body.appendChild(script); });
    commerceAdminLoaded = true;
  })();
  try { await commerceAdminLoading; } finally { commerceAdminLoading = null; }
}

async function openCommerceTab(name, element) {
  switchTab(name, element);
  await loadCommerceAdmin();
  if (name === 'commerce-orders' && typeof loadOrders === 'function') await loadOrders();
  if (name === 'commerce-channels' && typeof loadChannels === 'function') await loadChannels();
  if (name === 'commerce-settings' && typeof loadCommerceSettings === 'function') await loadCommerceSettings();
}
