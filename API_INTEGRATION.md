# 账号导入接口对接文档

## 接口概述

KiroClaim 提供了账号导入 API，支持批量导入 AWS 账号到账号池。你的注册账号系统可以通过这个接口自动将新注册的账号同步过来。

## 接口信息

### 1. 导入账号接口

**接口地址**：`POST /admin/accounts/import`

**认证方式**：需要在 Header 中携带管理员认证信息（Basic Auth 或 Session）

**请求格式**：`application/json`

**请求体结构**：

```json
[
  {
    "refreshToken": "aorAAAAAGsBe20...",
    "accessToken": "aoaAAAAAGqK90...",
    "clientId": "cSTBkueixbIuMka6jh95QnVzLWVhc3QtMQ",
    "clientSecret": "eyJraWQiOiJrZXktMT...",
    "provider": "idc",
    "region": "us-east-1"
  }
]
```

**必填字段**：
- `refreshToken`（必需）：AWS 刷新令牌

**可选字段**：
- `accessToken`：访问令牌（建议提供）
- `clientId`：客户端 ID
- `clientSecret`：客户端密钥
- `provider`：提供商标识（如：idc）
- `region`：区域（如：us-east-1）

**响应格式**：

```json
{
  "code": 0,
  "message": "导入任务已启动",
  "data": {
    "taskId": "k8m9n2p3q",
    "total": 10
  }
}
```

### 2. 查询导入状态接口

**接口地址**：`GET /admin/accounts/import/status/:taskId`

**响应格式**：

```json
{
  "code": 0,
  "data": {
    "taskId": "k8m9n2p3q",
    "status": "completed",
    "total": 10,
    "processed": 10,
    "imported": 8,
    "skippedDup": 2,
    "skippedBad": 0,
    "badDetails": [],
    "startTime": "2026-08-24T10:30:00Z",
    "endTime": "2026-08-24T10:30:15Z"
  }
}
```

**状态说明**：
- `processing`：导入中
- `completed`：导入完成
- `failed`：导入失败

## 接口特性

### 1. 自动去重
- 根据 `refreshToken` 自动去重
- 批内去重：同一批次中重复的账号只处理一次
- 库内去重：已存在的账号会跳过

### 2. 健康检查
- 自动验证账号有效性
- 并发健康检查（默认 6 个并发）
- 只有健康检查通过的账号才会导入

### 3. 异步处理
- 导入立即返回 taskId
- 后台异步处理导入
- 可通过 taskId 查询进度

### 4. 批量写入
- 数据库批量插入（每批 25 条）
- 提升导入性能

## 集成方案

### 方案一：推送模式（推荐）

你的注册系统在用户注册成功后，主动推送账号到 KiroClaim。

```python
import requests
import json

# KiroClaim 配置
KIROCLAIM_URL = "http://your-kiroclaim-server.com"
ADMIN_USERNAME = "admin"
ADMIN_PASSWORD = "your_password"

def import_account_to_kiroclaim(account_data):
    """
    将单个账号导入到 KiroClaim
    
    Args:
        account_data: dict，账号信息
            {
                'refreshToken': 'aor...',
                'accessToken': 'aoa...',
                'clientId': 'cST...',
                'clientSecret': 'eyJ...',
                'provider': 'idc',
                'region': 'us-east-1'
            }
    
    Returns:
        dict: 导入结果
    """
    
    # 构造请求数据（数组格式）
    payload = [account_data]
    
    # 发送导入请求
    response = requests.post(
        f"{KIROCLAIM_URL}/admin/accounts/import",
        json=payload,
        auth=(ADMIN_USERNAME, ADMIN_PASSWORD),
        headers={'Content-Type': 'application/json'}
    )
    
    if response.status_code != 200:
        return {
            'success': False,
            'error': f'HTTP {response.status_code}: {response.text}'
        }
    
    result = response.json()
    if result.get('code') != 0:
        return {
            'success': False,
            'error': result.get('message', '未知错误')
        }
    
    task_id = result['data']['taskId']
    
    # 等待导入完成（可选）
    import time
    max_wait = 30  # 最多等待 30 秒
    for _ in range(max_wait):
        status_response = requests.get(
            f"{KIROCLAIM_URL}/admin/accounts/import/status/{task_id}",
            auth=(ADMIN_USERNAME, ADMIN_PASSWORD)
        )
        
        if status_response.status_code == 200:
            status_data = status_response.json()
            if status_data['code'] == 0:
                task_status = status_data['data']['status']
                
                if task_status == 'completed':
                    return {
                        'success': True,
                        'taskId': task_id,
                        'imported': status_data['data']['imported'],
                        'skipped': status_data['data']['skippedDup'] + status_data['data']['skippedBad']
                    }
                elif task_status == 'failed':
                    return {
                        'success': False,
                        'error': '导入任务失败'
                    }
        
        time.sleep(1)
    
    # 超时
    return {
        'success': False,
        'error': '导入超时，请稍后查询状态'
    }


def batch_import_accounts(accounts_list):
    """
    批量导入账号
    
    Args:
        accounts_list: list，账号列表
    
    Returns:
        dict: 导入结果
    """
    
    response = requests.post(
        f"{KIROCLAIM_URL}/admin/accounts/import",
        json=accounts_list,
        auth=(ADMIN_USERNAME, ADMIN_PASSWORD),
        headers={'Content-Type': 'application/json'}
    )
    
    if response.status_code != 200:
        return {
            'success': False,
            'error': f'HTTP {response.status_code}: {response.text}'
        }
    
    result = response.json()
    if result.get('code') != 0:
        return {
            'success': False,
            'error': result.get('message', '未知错误')
        }
    
    return {
        'success': True,
        'taskId': result['data']['taskId'],
        'total': result['data']['total']
    }


# 使用示例
if __name__ == '__main__':
    # 示例：注册成功后导入账号
    new_account = {
        'refreshToken': 'aorAAAAAGsBe20EWyIQBTqsxtwHcjZZQy-16c5kfpMxZtjnL17s...',
        'accessToken': 'aoaAAAAAGqK90AXEkG6ByWyENlWQxv4naFrACAMz4aAHkUYnEg-...',
        'clientId': 'cSTBkueixbIuMka6jh95QnVzLWVhc3QtMQ',
        'clientSecret': 'eyJraWQiOiJrZXktMTU2NDAyODA5OSIsImFsZyI6IkhTMzg0In0...',
        'provider': 'idc',
        'region': 'us-east-1'
    }
    
    result = import_account_to_kiroclaim(new_account)
    print(f"导入结果: {result}")
```

### 方案二：拉取模式

KiroClaim 定时从你的注册系统拉取新账号。

```go
// 在 KiroClaim 中添加定时任务
package handler

import (
    "encoding/json"
    "net/http"
    "time"
)

type ExternalAccount struct {
    RefreshToken  string `json:"refreshToken"`
    AccessToken   string `json:"accessToken"`
    ClientId      string `json:"clientId"`
    ClientSecret  string `json:"clientSecret"`
    Provider      string `json:"provider"`
    Region        string `json:"region"`
}

// 从外部系统获取新账号
func fetchAccountsFromExternalSystem() ([]map[string]interface{}, error) {
    // 配置外部系统 API
    externalAPIURL := "http://your-register-system.com/api/accounts/new"
    externalAPIKey := "your_api_key"
    
    client := &http.Client{Timeout: 30 * time.Second}
    req, err := http.NewRequest("GET", externalAPIURL, nil)
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", "Bearer "+externalAPIKey)
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var accounts []map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&accounts); err != nil {
        return nil, err
    }
    
    return accounts, nil
}

// 定时同步任务
func StartAccountSyncScheduler() {
    ticker := time.NewTicker(5 * time.Minute) // 每 5 分钟同步一次
    
    go func() {
        for range ticker.C {
            accounts, err := fetchAccountsFromExternalSystem()
            if err != nil {
                log.Printf("获取外部账号失败: %v", err)
                continue
            }
            
            if len(accounts) == 0 {
                continue
            }
            
            // 调用导入逻辑
            taskID := strconv.FormatInt(time.Now().UnixNano(), 36)
            go processImport(taskID, accounts)
            
            log.Printf("同步了 %d 个账号，taskID: %s", len(accounts), taskID)
        }
    }()
}
```

### 方案三：Webhook 回调

你的注册系统在账号创建时，通过 Webhook 通知 KiroClaim。

```javascript
// 你的注册系统中的 Webhook 触发代码
async function onAccountRegistered(accountData) {
    const webhookURL = 'http://your-kiroclaim-server.com/admin/accounts/import';
    const credentials = Buffer.from('admin:your_password').toString('base64');
    
    try {
        const response = await fetch(webhookURL, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Basic ${credentials}`
            },
            body: JSON.stringify([{
                refreshToken: accountData.refreshToken,
                accessToken: accountData.accessToken,
                clientId: accountData.clientId,
                clientSecret: accountData.clientSecret,
                provider: accountData.provider || 'idc',
                region: accountData.region || 'us-east-1'
            }])
        });
        
        const result = await response.json();
        
        if (result.code === 0) {
            console.log('账号导入成功:', result.data.taskId);
            return true;
        } else {
            console.error('账号导入失败:', result.message);
            return false;
        }
    } catch (error) {
        console.error('导入请求失败:', error);
        return false;
    }
}
```

## 错误处理

### 常见错误码

| 错误码 | 说明 | 处理方式 |
|--------|------|----------|
| 1 | JSON 格式错误 | 检查请求体格式是否正确 |
| 401 | 未授权 | 检查认证信息是否正确 |
| 404 | 任务不存在 | 检查 taskId 是否正确 |

### 导入失败原因

查看导入状态接口的 `badDetails` 字段：

```json
{
  "badDetails": [
    {
      "row": 1,
      "reason": "缺少 refreshToken"
    },
    {
      "row": 3,
      "reason": "刷新 token 403: 账号已封禁"
    }
  ]
}
```

## 最佳实践

### 1. 使用推送模式
- 实时性更好
- 减少系统负担
- 便于跟踪每个账号的导入状态

### 2. 批量导入优化
- 建议每批 50-100 个账号
- 避免单次导入过多导致超时

### 3. 错误重试
```python
def import_with_retry(account_data, max_retries=3):
    for attempt in range(max_retries):
        result = import_account_to_kiroclaim(account_data)
        if result['success']:
            return result
        
        if attempt < max_retries - 1:
            time.sleep(2 ** attempt)  # 指数退避
    
    return result
```

### 4. 日志记录
- 记录每次导入的 taskId
- 保存导入失败的账号信息
- 定期清理成功导入的日志

### 5. 监控告警
- 监控导入成功率
- 导入失败超过阈值时告警
- 定期检查 `skippedBad` 数量

## 安全建议

1. **使用 HTTPS**：生产环境必须使用 HTTPS 加密传输
2. **认证凭证保护**：不要在代码中硬编码密码，使用环境变量
3. **IP 白名单**：在 KiroClaim 中配置允许的 IP 地址
4. **限流保护**：设置合理的导入频率限制
5. **日志脱敏**：记录日志时避免输出完整的 token

## 测试示例

```bash
# 使用 curl 测试导入接口
curl -X POST http://localhost:8080/admin/accounts/import \
  -u admin:password \
  -H "Content-Type: application/json" \
  -d '[
    {
      "refreshToken": "aorAAAAAGsBe20...",
      "accessToken": "aoaAAAAAGqK90...",
      "clientId": "cSTBkueixbIuMka6jh95QnVzLWVhc3QtMQ",
      "clientSecret": "eyJraWQiOiJrZXktMT..."
    }
  ]'

# 查询导入状态
curl http://localhost:8080/admin/accounts/import/status/k8m9n2p3q \
  -u admin:password
```

## 联系支持

如需进一步的技术支持或定制开发，请联系开发团队。
