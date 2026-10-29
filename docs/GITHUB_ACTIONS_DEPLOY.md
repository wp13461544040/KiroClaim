# KiroClaim 双环境自动部署配置指南

## 🎯 概述

KiroClaim 项目配置了自动部署到两个生产环境：
- **国内环境**: 111.229.254.24
- **US 环境**: 64.83.17.68

当代码推送到 `main` 分支并成功构建 Docker 镜像后，会自动触发两个环境的部署。

---

## 📋 部署流程

```
推送代码到 main 分支
        ↓
   构建 Docker 镜像
        ↓
   上传到 GHCR
        ↓
  ✅ Docker 构建成功
        ↓
    自动触发部署
        ↓
    ┌─────────┬─────────┐
    ↓         ↓         ↓
国内环境   US环境   部署通知
```

---

## 🔐 第一步：配置 GitHub Secrets

### 进入配置页面
1. 打开仓库: https://github.com/wp13461544040/KiroClaim
2. 点击 **Settings** (设置)
3. 左侧菜单选择 **Secrets and variables** → **Actions**
4. 点击 **New repository secret** 添加配置

### 需要配置的 Secrets（共 8 个）

#### 国内环境（4 个）

| Secret 名称 | 值 | 说明 |
|------------|-----|------|
| `DEPLOY_CHINA_HOST` | `111.229.254.24` | 国内服务器 IP 地址 |
| `DEPLOY_CHINA_USER` | `root` | SSH 登录用户名 |
| `DEPLOY_CHINA_PASSWORD` | `Wp156174225.` | SSH 登录密码 |
| `DEPLOY_CHINA_PORT` | `22` | SSH 端口（可选，默认 22） |

#### US 环境（4 个）

| Secret 名称 | 值 | 说明 |
|------------|-----|------|
| `DEPLOY_US_HOST` | `64.83.17.68` | US 服务器 IP 地址 |
| `DEPLOY_US_USER` | `root` | SSH 登录用户名 |
| `DEPLOY_US_PASSWORD` | `Wp156174225.` | SSH 登录密码 |
| `DEPLOY_US_PORT` | `22` | SSH 端口（可选，默认 22） |

### 快速配置脚本（使用 GitHub CLI）

如果你安装了 `gh` 命令行工具：

```bash
# 确保已登录
gh auth login

# 切换到 KiroClaim 项目目录
cd ~/KiroClaim  # 或你的本地项目路径

# 配置国内环境
gh secret set DEPLOY_CHINA_HOST --body "111.229.254.24"
gh secret set DEPLOY_CHINA_USER --body "root"
gh secret set DEPLOY_CHINA_PASSWORD --body "Wp156174225."
gh secret set DEPLOY_CHINA_PORT --body "22"

# 配置 US 环境
gh secret set DEPLOY_US_HOST --body "64.83.17.68"
gh secret set DEPLOY_US_USER --body "root"
gh secret set DEPLOY_US_PASSWORD --body "Wp156174225."
gh secret set DEPLOY_US_PORT --body "22"

# 验证配置
gh secret list
```

---

## 🚀 第二步：部署方式

### 方式一：自动部署（推荐）

推送代码到 `main` 分支，自动触发：

```bash
git add .
git commit -m "feat: 新功能"
git push origin main
```

部署流程：
1. GitHub Actions 构建 Docker 镜像
2. 镜像构建成功后自动触发部署 workflow
3. 并行部署到国内和 US 两个环境
4. 显示部署结果通知

### 方式二：手动部署

1. 进入 GitHub Actions 页面
   - https://github.com/wp13461544040/KiroClaim/actions
2. 选择左侧 **Deploy to Dual Environments**
3. 点击右上角 **Run workflow** 按钮
4. 选择部署选项：
   - `both` - 同时部署到国内和 US（默认）
   - `china` - 仅部署到国内环境
   - `us` - 仅部署到 US 环境
5. 点击 **Run workflow** 开始

使用 GitHub CLI 手动触发：
```bash
# 部署到两个环境
gh workflow run deploy-dual-env.yml -f environment=both

# 仅部署国内
gh workflow run deploy-dual-env.yml -f environment=china

# 仅部署 US
gh workflow run deploy-dual-env.yml -f environment=us
```

---

## 📦 部署执行的命令

### 国内环境部署步骤

服务器路径: `/home/ubuntu/kiroclaim/`

```bash
cd /home/ubuntu/kiroclaim/
docker compose ps                    # 检查容器状态
docker compose pull kiroclaim        # 拉取最新镜像
docker compose up -d kiroclaim       # 启动容器
docker image prune -f                # 清理未使用的镜像
docker restart kiroclaim             # 重启容器确保更新
```

### US 环境部署步骤

服务器路径: `~/kiroclaim`

```bash
cd ~/kiroclaim
docker pull ghcr.io/wp13461544040/kiroclaim:latest  # 拉取最新镜像
docker compose up -d                                # 启动所有容器
docker image prune -f                               # 清理未使用的镜像
docker restart kiroclaim                            # 重启容器确保更新
```

---

## 🔍 第三步：验证部署

### 查看 GitHub Actions 日志

1. 进入 Actions 页面查看执行状态
2. 点击最新的 workflow run
3. 查看每个 job 的详细日志
4. 确认部署成功完成

### 登录服务器验证

```bash
# SSH 登录国内服务器
ssh root@111.229.254.24

# 检查容器状态
cd /home/ubuntu/kiroclaim/
docker ps | grep kiroclaim
docker logs kiroclaim --tail 50

# 测试服务
curl http://localhost:9527/health
```

```bash
# SSH 登录 US 服务器
ssh root@64.83.17.68

# 检查容器状态
cd ~/kiroclaim
docker ps | grep kiroclaim
docker logs kiroclaim --tail 50

# 测试服务
curl http://localhost:9527/health
```

### 浏览器访问验证

- 国内环境: http://111.229.254.24:9527
- US 环境: http://64.83.17.68:9527

---

## 🛡️ 安全建议

### ⚠️ 密码安全

当前配置使用密码认证，存在以下风险：
- 密码泄露风险
- 密码重用风险
- 暴力破解风险

### ✅ 推荐升级到 SSH 密钥认证

#### 1. 生成 SSH 密钥对

```bash
# 生成专用于部署的 SSH 密钥
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/kiroclaim_deploy
```

这会生成两个文件：
- `kiroclaim_deploy` - 私钥（保密）
- `kiroclaim_deploy.pub` - 公钥（上传到服务器）

#### 2. 添加公钥到服务器

```bash
# 查看公钥内容
cat ~/.ssh/kiroclaim_deploy.pub

# 复制输出的内容，然后登录到每个服务器
ssh root@111.229.254.24

# 在服务器上执行
mkdir -p ~/.ssh
chmod 700 ~/.ssh
echo "公钥内容" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
exit

# 对 US 服务器重复上述步骤
ssh root@64.83.17.68
# ... 相同操作
```

#### 3. 测试 SSH 密钥登录

```bash
# 测试国内服务器
ssh -i ~/.ssh/kiroclaim_deploy root@111.229.254.24

# 测试 US 服务器
ssh -i ~/.ssh/kiroclaim_deploy root@64.83.17.68
```

#### 4. 更新 GitHub Secrets

```bash
# 读取私钥内容
cat ~/.ssh/kiroclaim_deploy

# 在 GitHub Secrets 中：
# 1. 删除 DEPLOY_CHINA_PASSWORD 和 DEPLOY_US_PASSWORD
# 2. 添加新的 Secrets：
gh secret set DEPLOY_CHINA_KEY < ~/.ssh/kiroclaim_deploy
gh secret set DEPLOY_US_KEY < ~/.ssh/kiroclaim_deploy
```

#### 5. 修改 Workflow 文件

编辑 `.github/workflows/deploy-dual-env.yml`：

```yaml
# 国内环境部署 - 将 password 改为 key
- name: 部署到国内环境
  uses: appleboy/ssh-action@v1.2.0
  with:
    host: ${{ secrets.DEPLOY_CHINA_HOST }}
    port: ${{ secrets.DEPLOY_CHINA_PORT || 22 }}
    username: ${{ secrets.DEPLOY_CHINA_USER }}
    key: ${{ secrets.DEPLOY_CHINA_KEY }}  # 使用密钥
    script_stop: true
    command_timeout: 30m
    script: |
      # ... 部署脚本保持不变

# US 环境部署 - 将 password 改为 key
- name: 部署到 US 环境
  uses: appleboy/ssh-action@v1.2.0
  with:
    host: ${{ secrets.DEPLOY_US_HOST }}
    port: ${{ secrets.DEPLOY_US_PORT || 22 }}
    username: ${{ secrets.DEPLOY_US_USER }}
    key: ${{ secrets.DEPLOY_US_KEY }}  # 使用密钥
    script_stop: true
    command_timeout: 30m
    script: |
      # ... 部署脚本保持不变
```

#### 6. 服务器安全加固

```bash
# 禁用密码登录（推荐）
sudo nano /etc/ssh/sshd_config

# 修改以下配置
PasswordAuthentication no
PubkeyAuthentication yes
PermitRootLogin prohibit-password

# 重启 SSH 服务
sudo systemctl restart sshd
```

---

## ⚙️ 高级配置

### 配置部署通知（可选）

可以添加企业微信、钉钉、Slack 等通知，在部署完成时发送消息。

#### 企业微信机器人示例

在 workflow 的 `notify` job 中添加：

```yaml
- name: 发送企业微信通知
  if: always()
  run: |
    WEBHOOK_URL="${{ secrets.WECHAT_WEBHOOK_URL }}"
    if [ -z "$WEBHOOK_URL" ]; then
      echo "未配置企业微信 webhook，跳过通知"
      exit 0
    fi
    
    STATUS="✅ 成功"
    if [[ "${{ needs.deploy-china.result }}" == "failure" ]] || [[ "${{ needs.deploy-us.result }}" == "failure" ]]; then
      STATUS="❌ 失败"
    fi
    
    curl "$WEBHOOK_URL" \
      -H 'Content-Type: application/json' \
      -d "{
        \"msgtype\": \"text\",
        \"text\": {
          \"content\": \"KiroClaim 部署通知\n状态: $STATUS\n国内: ${{ needs.deploy-china.result }}\nUS: ${{ needs.deploy-us.result }}\n提交: ${{ github.sha }}\n作者: ${{ github.actor }}\"
        }
      }"
```

### 配置环境保护规则（可选）

为防止误操作，可以在 GitHub 仓库设置中配置环境保护：

1. 进入 **Settings** → **Environments**
2. 添加环境 `production-china` 和 `production-us`
3. 配置保护规则：
   - ✅ Required reviewers - 需要审批才能部署
   - ✅ Wait timer - 延迟部署（例如 5 分钟）
   - ✅ Deployment branches - 限制只能从 `main` 分支部署

---

## 🐛 故障排查

### 问题 1: 部署失败 - SSH 连接超时

**原因**: 服务器防火墙阻止或 IP 被封禁

**解决**:
```bash
# 检查服务器 SSH 服务状态
ssh root@服务器IP
sudo systemctl status sshd

# 检查防火墙规则
sudo ufw status
sudo ufw allow 22/tcp

# 查看 SSH 登录日志
sudo tail -f /var/log/auth.log
```

### 问题 2: 部署失败 - 镜像拉取失败

**原因**: 服务器无法访问 GitHub Container Registry

**解决**:
```bash
# 测试网络连接
ping ghcr.io
curl -I https://ghcr.io

# 手动拉取测试
docker pull ghcr.io/wp13461544040/kiroclaim:latest

# 如果是私有镜像，需要登录
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

### 问题 3: 容器启动失败

**原因**: 配置错误或端口占用

**解决**:
```bash
# 查看容器日志
docker logs kiroclaim

# 检查端口占用
sudo netstat -tulpn | grep 9527
sudo lsof -i :9527

# 检查配置文件
cd ~/kiroclaim
cat .env
cat docker-compose.yml

# 重新启动
docker compose down
docker compose up -d
```

### 问题 4: Secrets 配置错误

**症状**: GitHub Actions 显示 `secret not found`

**解决**:
```bash
# 验证 secrets 配置
gh secret list

# 重新配置缺失的 secret
gh secret set SECRET_NAME --body "value"
```

### 问题 5: 权限不足

**症状**: `Permission denied` 或 `sudo: a password is required`

**解决**:
- 确保使用 `root` 用户
- 或者为普通用户配置无密码 sudo：
```bash
sudo visudo
# 添加：username ALL=(ALL) NOPASSWD:ALL
```

---

## 📊 监控和日志

### GitHub Actions 日志

- 每次部署都有详细日志记录
- 保留 90 天（GitHub 默认）
- 可以下载日志文件进行分析

### 服务器日志

```bash
# 查看应用日志
docker logs kiroclaim --tail 100 -f

# 查看 Docker Compose 日志
cd ~/kiroclaim
docker compose logs -f

# 查看系统日志
sudo journalctl -u docker -f
```

### 部署历史记录

```bash
# 查看最近的 workflow 运行
gh run list --workflow=deploy-dual-env.yml --limit 10

# 查看特定运行的详细信息
gh run view RUN_ID

# 查看运行日志
gh run view RUN_ID --log
```

---

## 📝 维护清单

### 每月检查
- [ ] 检查服务器磁盘空间
- [ ] 清理 Docker 未使用的镜像和容器
- [ ] 检查部署日志是否有异常
- [ ] 验证两个环境的服务可访问性

### 每季度检查
- [ ] 更新服务器系统包
- [ ] 更新 Docker 和 Docker Compose
- [ ] 审查和更新 SSH 密钥
- [ ] 检查并更新部署脚本

### 年度检查
- [ ] 轮换所有密码和密钥
- [ ] 审查访问权限
- [ ] 备份重要数据
- [ ] 更新文档

---

## 📖 相关文档

- [KiroClaim 部署文档](./DEPLOYMENT.md) - 完整的服务器部署指南
- [Docker Workflow](.github/workflows/docker.yml) - Docker 镜像构建配置
- [部署 Workflow](.github/workflows/deploy-dual-env.yml) - 双环境部署配置

---

## 🎉 完成配置

配置完成后的首次测试步骤：

1. ✅ 确认所有 8 个 GitHub Secrets 已配置
2. ✅ 手动触发一次部署测试
3. ✅ 检查 GitHub Actions 日志确认成功
4. ✅ 登录两个服务器验证服务运行正常
5. ✅ 浏览器访问验证页面可正常打开
6. ✅ 推送一次代码触发自动部署测试

**🎊 恭喜！双环境自动部署配置完成！**

---

**文档版本**: v1.0  
**更新日期**: 2026-10-29  
**适用版本**: KiroClaim v0.1.0+
