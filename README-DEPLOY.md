# KiroClaim Windows 部署指南

## ⚡ 一键部署

**右键以管理员身份运行：**
```cmd
one-click-deploy.bat
```

等待 3-5 分钟，访问：http://localhost:9527/setup

---

## 📋 部署说明

脚本自动执行：
1. 检查并安装 Go 1.24.0（如需要）
2. 创建配置文件（生成随机 JWT 密钥）
3. 下载依赖并编译
4. 配置防火墙（端口 9527）
5. 启动应用服务

---

## 🌐 访问地址

| 功能 | URL |
|------|-----|
| 初始化 | http://localhost:9527/setup |
| 管理后台 | http://localhost:9527/admin |
| 兑换中心 | http://localhost:9527/redeem |
| 商城首页 | http://localhost:9527/ |

---

## 🛠️ 管理命令

```powershell
# 查看进程
Get-Process -Name kiroclaim

# 查看日志
Get-Content logs\app.log -Tail 50 -Wait

# 停止服务
Stop-Process -Name kiroclaim

# 重启服务
Stop-Process -Name kiroclaim
Start-Sleep -Seconds 2
Start-Process -FilePath ".\kiroclaim.exe" -WindowStyle Hidden
```

---

## ❓ 常见问题

**Q: 端口被占用**
```powershell
netstat -ano | findstr :9527
Stop-Process -Id <PID> -Force
```

**Q: 无法访问**
- 检查进程：`Get-Process -Name kiroclaim`
- 检查防火墙：`Get-NetFirewallRule -DisplayName "KiroClaim-9527"`
- 检查端口：`netstat -ano | findstr :9527`

**Q: Go 安装失败**
手动下载：https://go.dev/dl/go1.24.0.windows-amd64.msi

---

## 🔒 生产建议

1. **修改 JWT 密钥**（脚本已自动生成）
2. **限制访问 IP**
   ```powershell
   New-NetFirewallRule -DisplayName "KiroClaim-9527" -Direction Inbound -Protocol TCP -LocalPort 9527 -Action Allow -RemoteAddress "允许的IP"
   ```
3. **配置 HTTPS**（使用 Nginx/IIS 反向代理）
4. **定期备份**
   ```powershell
   Copy-Item app.db "app.db.backup.$(Get-Date -Format 'yyyyMMdd')"
   ```

---

**部署完成！** 🎉

## 快速开始

### 1. 一键部署（推荐）

双击运行 `deploy-windows.bat` 即可自动完成所有部署步骤。

该脚本会：
- ✅ 检查 Go 环境
- ✅ 验证必需文件
- ✅ 创建配置文件
- ✅ 下载依赖包
- ✅ 编译应用程序
- ✅ 配置防火墙

### 2. 启动应用

#### 方式一：前台运行（推荐用于开发/测试）
```batch
start.bat
```
或直接运行：
```batch
.\kiroclaim.exe
```

#### 方式二：后台运行（推荐用于生产）
```powershell
.\start-background.ps1
```

### 3. 访问应用

部署完成后，通过浏览器访问：

- **首次设置**: http://localhost:9527/setup
- **管理后台**: http://localhost:9527/admin
- **兑换页面**: http://localhost:9527/redeem
- **商城首页**: http://localhost:9527/

## 管理命令

### 查看状态
```powershell
.\status.ps1
```
显示应用运行状态、资源占用、日志等信息。

### 停止服务
```powershell
.\stop.ps1
```
或者：
```powershell
Stop-Process -Name kiroclaim
```

### 查看日志
```powershell
# 查看最新50行
Get-Content logs\app.log -Tail 50

# 实时监控日志
Get-Content logs\app.log -Tail 50 -Wait
```

### 重启服务
```powershell
.\stop.ps1
.\start-background.ps1
```

## 系统要求

### 必需软件
- **Windows 10/11** 或 Windows Server 2016+
- **Go 1.21+** ([下载地址](https://go.dev/dl/))
  - 推荐：go1.24.0.windows-amd64.msi

### 可选软件
- **Git** - 用于版本控制（部署脚本会显示 Git 提交信息）

### 硬件要求
- **CPU**: 1核心以上
- **内存**: 512MB 以上
- **磁盘**: 100MB 可用空间

## 配置说明

### 环境变量（.env 文件）

首次部署会自动从 `.env.example` 创建 `.env` 文件，主要配置项：

```bash
# 服务端口
PORT=9527

# JWT 密钥（建议修改为随机字符串）
JWT_SECRET=your-secret-key-here

# 数据库路径
DATABASE_PATH=./app.db

# 日志级别 (debug, info, warn, error)
LOG_LEVEL=info
```

### 防火墙配置

部署脚本会自动配置防火墙规则，允许端口 9527 的入站连接。

如需手动配置：
```powershell
# 添加防火墙规则
New-NetFirewallRule -DisplayName "KiroClaim-9527" -Direction Inbound -Protocol TCP -LocalPort 9527 -Action Allow

# 查看规则
Get-NetFirewallRule -DisplayName "KiroClaim-9527"

# 删除规则
Remove-NetFirewallRule -DisplayName "KiroClaim-9527"
```

## 故障排查

### 问题1：找不到 Go 命令
**症状**: `go: command not found` 或 `Go not found`

**解决方案**:
1. 确认已安装 Go: https://go.dev/dl/
2. 检查环境变量：
   - 确保 `C:\go\bin` 在 PATH 中
   - 或安装到默认路径 `C:\go`

### 问题2：端口被占用
**症状**: `bind: address already in use` 或端口 9527 无法监听

**解决方案**:
```powershell
# 查看端口占用
netstat -ano | findstr :9527

# 停止占用端口的进程
Stop-Process -Id <PID> -Force
```

或修改 `.env` 文件中的 `PORT` 配置。

### 问题3：依赖下载失败
**症状**: `go mod download` 失败

**解决方案**:
```powershell
# 设置国内代理
$env:GOPROXY = "https://goproxy.cn,direct"

# 重新下载
go mod download
```

### 问题4：无法删除旧的 exe 文件
**症状**: `The process cannot access the file because it is being used`

**解决方案**:
```powershell
# 停止所有 kiroclaim 进程
Stop-Process -Name kiroclaim -Force

# 等待几秒后重新部署
Start-Sleep -Seconds 2
.\deploy-windows.bat
```

### 问题5：防火墙配置失败
**症状**: `Firewall rule configuration failed`

**解决方案**:
1. 以管理员身份运行部署脚本：
   - 右键 `deploy-windows.bat`
   - 选择"以管理员身份运行"

2. 手动配置防火墙（见上方"防火墙配置"章节）

## 更新应用

### 方式一：重新部署
```batch
.\deploy-windows.bat
```

### 方式二：手动编译
```powershell
# 停止服务
.\stop.ps1

# 拉取最新代码（如果使用 Git）
git pull

# 下载依赖
go mod download

# 编译
go build -o kiroclaim.exe .

# 启动服务
.\start-background.ps1
```

## 生产环境建议

1. **修改默认密钥**
   - 编辑 `.env` 文件
   - 修改 `JWT_SECRET` 为强随机字符串

2. **定期备份数据库**
   ```powershell
   # 创建备份
   Copy-Item app.db "app.db.backup.$(Get-Date -Format 'yyyyMMdd-HHmmss')"
   ```

3. **启用 HTTPS**
   - 使用反向代理（如 Nginx）
   - 或配置应用的 TLS 证书

4. **设置日志轮转**
   - 防止日志文件过大
   - 定期清理旧日志

5. **监控服务运行**
   - 使用 `status.ps1` 定期检查
   - 配置自动重启机制

6. **限制访问权限**
   - 配置防火墙规则
   - 只允许必要的 IP 访问

## 开机自启动

### 方式一：任务计划程序
1. 打开"任务计划程序"
2. 创建基本任务
3. 触发器：系统启动时
4. 操作：启动程序 `powershell.exe`
5. 参数：`-ExecutionPolicy Bypass -File "C:\path\to\start-background.ps1"`

### 方式二：使用 NSSM（推荐）
```powershell
# 下载 NSSM: https://nssm.cc/download

# 安装服务
nssm install KiroClaim "C:\path\to\kiroclaim.exe"

# 配置服务
nssm set KiroClaim AppDirectory "C:\path\to\project"
nssm set KiroClaim DisplayName "KiroClaim Service"
nssm set KiroClaim Description "KiroClaim Application Service"

# 启动服务
nssm start KiroClaim
```

## 文件结构

```
KiroClaim/
├── kiroclaim.exe              # 主程序
├── app.db                     # SQLite 数据库
├── .env                       # 环境变量配置
├── logs/
│   └── app.log               # 应用日志
├── static/                   # 静态资源
├── deploy-windows.bat        # 部署脚本（入口）
├── deploy-windows.ps1        # 部署脚本（主逻辑）
├── start.bat                 # 前台启动脚本
├── start-background.ps1      # 后台启动脚本
├── stop.ps1                  # 停止脚本
├── status.ps1                # 状态检查脚本
└── README-DEPLOY.md          # 本文档
```

## 技术支持

如遇到其他问题，请：
1. 查看日志文件 `logs\app.log`
2. 运行 `status.ps1` 检查状态
3. 查阅项目文档或提交 Issue

---

**部署脚本版本**: 1.1.0  
**最后更新**: 2026-07-25
