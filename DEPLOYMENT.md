# KiroClaim Ubuntu服务器Docker镜像部署文档

> **部署方式**：使用GitHub Container Registry的Docker镜像部署，无需上传代码  
> **镜像地址**：`ghcr.io/wp13461544040/kiroclaim:latest`  
> **部署时间**：约5-10分钟

---

## 一、环境要求

### 1.1 服务器配置
- **操作系统**：Ubuntu 20.04+ / Debian 11+
- **CPU**：1核心+
- **内存**：1GB+
- **磁盘**：10GB+
- **网络**：可访问GitHub Container Registry (ghcr.io)

### 1.2 必需软件
- Docker 20.10+
- Docker Compose V2

---

## 二、服务器准备（全新Ubuntu服务器）

### 2.1 更新系统包
```bash
sudo apt update && sudo apt upgrade -y
```

### 2.2 安装Docker
```bash
# 安装依赖
sudo apt install -y ca-certificates curl gnupg lsb-release

# 添加Docker官方GPG密钥
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg

# 添加Docker仓库
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# 安装Docker Engine
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# 验证安装
docker --version
docker compose version
```

### 2.3 配置Docker权限（可选）
```bash
# 将当前用户添加到docker组，避免每次使用sudo
sudo usermod -aG docker $USER

# 重新登录以生效，或执行：
newgrp docker

# 测试
docker ps
```

### 2.4 启动Docker服务
```bash
sudo systemctl enable docker
sudo systemctl start docker
sudo systemctl status docker
```

---

## 三、部署KiroClaim

### 3.1 创建项目目录
```bash
# 创建部署目录
mkdir -p ~/kiroclaim && cd ~/kiroclaim
```

### 3.2 下载docker-compose.yml
```bash
# 方式1：使用curl下载（推荐）
curl -fsSL https://raw.githubusercontent.com/wp13461544040/KiroClaim/main/docker-compose.yml -o docker-compose.yml

# 方式2：使用wget下载
wget https://raw.githubusercontent.com/wp13461544040/KiroClaim/main/docker-compose.yml

# 验证下载
cat docker-compose.yml
```

### 3.3 创建环境变量文件
```bash
# 生成强随机密码并创建.env文件
cat > .env << EOF
PORT=9527
MYSQL_DATABASE=kiroclaim
MYSQL_USER=kiroclaim
MYSQL_PASSWORD=$(openssl rand -hex 24)
MYSQL_ROOT_PASSWORD=$(openssl rand -hex 24)
EOF

# 查看生成的配置
cat .env
```

**重要提示**：
- 请保存好`.env`文件中的密码，遗失后无法找回
- 生产环境建议修改默认端口`9527`为其他端口

### 3.4 拉取Docker镜像
```bash
# 拉取最新版本镜像
docker pull ghcr.io/wp13461544040/kiroclaim:latest

# 拉取指定版本镜像（例如v0.2.1-beta）
# docker pull ghcr.io/wp13461544040/kiroclaim:v0.2.1-beta

# 验证镜像
docker images | grep kiroclaim
```

### 3.5 启动服务
```bash
# 后台启动所有服务
docker compose up -d

# 查看启动日志
docker compose logs -f

# 等待MySQL健康检查通过（约10-30秒）
# 看到 "kiroclaim  | Server is running on port 9527" 表示启动成功
# 按 Ctrl+C 退出日志查看
```

### 3.6 验证服务状态
```bash
# 查看容器状态
docker compose ps

# 应该看到两个容器都是 "Up" 状态：
# kiroclaim        Up      0.0.0.0:9527->9527/tcp
# kiroclaim-mysql  Up      3306/tcp
```

---

## 四、访问与初始化

### 4.1 开放防火墙端口
```bash
# Ubuntu UFW防火墙
sudo ufw allow 9527/tcp
sudo ufw reload
sudo ufw status

# 如果使用云服务器（阿里云/腾讯云/AWS等）
# 需要在控制台的安全组中开放 9527 端口
```

### 4.2 访问系统
```bash
# 获取服务器公网IP
curl ifconfig.me

# 或
hostname -I
```

在浏览器访问：
- **初始化页面**：`http://服务器IP:9527/setup`
- **兑换中心**：`http://服务器IP:9527/redeem`
- **管理后台**：`http://服务器IP:9527/admin`（初始化后）

### 4.3 初始化系统
1. 访问 `http://服务器IP:9527/setup`
2. 按照页面提示创建管理员账号
3. 设置管理员用户名和密码
4. 完成初始化后会自动跳转到登录页

---

## 五、日常运维命令

### 5.1 查看服务状态
```bash
cd ~/kiroclaim

# 查看容器状态
docker compose ps

# 查看实时日志
docker compose logs -f

# 查看kiroclaim服务日志
docker compose logs -f kiroclaim

# 查看MySQL日志
docker compose logs -f mysql
```

### 5.2 重启服务
```bash
cd ~/kiroclaim

# 重启所有服务
docker compose restart

# 仅重启kiroclaim服务
docker compose restart kiroclaim

# 重启MySQL服务
docker compose restart mysql
```

### 5.3 停止服务
```bash
cd ~/kiroclaim

# 停止所有服务（保留数据）
docker compose stop

# 停止并删除容器（保留数据）
docker compose down

# 完全删除（包括数据卷，慎用！）
docker compose down -v
```

### 5.4 更新服务
```bash
cd ~/kiroclaim

# 拉取最新镜像
docker pull ghcr.io/wp13461544040/kiroclaim:latest

# 重新创建并启动容器
docker compose up -d

# 清理旧镜像
docker image prune -f
```

### 5.5 备份数据
```bash
cd ~/kiroclaim

# 备份MySQL数据
docker compose exec mysql mysqldump -u kiroclaim -p kiroclaim > backup_$(date +%Y%m%d_%H%M%S).sql

# 备份日志文件
docker compose cp kiroclaim:/app/logs ./logs_backup_$(date +%Y%m%d_%H%M%S)

# 备份整个数据卷
docker run --rm -v kiroclaim_mysql-data:/data -v $(pwd):/backup alpine tar czf /backup/mysql-data_$(date +%Y%m%d_%H%M%S).tar.gz -C /data .
```

### 5.6 恢复数据
```bash
cd ~/kiroclaim

# 恢复MySQL数据
docker compose exec -T mysql mysql -u kiroclaim -p kiroclaim < backup_20260108_120000.sql

# 恢复数据卷
docker run --rm -v kiroclaim_mysql-data:/data -v $(pwd):/backup alpine tar xzf /backup/mysql-data_20260108_120000.tar.gz -C /data
```

---

## 六、故障排查

### 6.1 容器启动失败
```bash
# 查看详细错误日志
docker compose logs

# 检查端口占用
sudo netstat -tulpn | grep 9527
sudo lsof -i :9527

# 检查磁盘空间
df -h

# 检查Docker状态
sudo systemctl status docker
```

### 6.2 MySQL连接失败
```bash
# 检查MySQL容器状态
docker compose ps mysql

# 查看MySQL日志
docker compose logs mysql

# 进入MySQL容器检查
docker compose exec mysql mysql -uroot -p
# 输入MYSQL_ROOT_PASSWORD（从.env文件查看）

# 测试数据库连接
docker compose exec mysql mysqladmin ping -h 127.0.0.1 -uroot -p
```

### 6.3 无法访问服务
```bash
# 检查容器是否运行
docker compose ps

# 检查端口映射
docker compose port kiroclaim 9527

# 检查防火墙
sudo ufw status
sudo ufw allow 9527/tcp

# 检查服务监听
docker compose exec kiroclaim netstat -tulpn | grep 9527

# 在服务器本地测试
curl http://localhost:9527/health
```

### 6.4 镜像拉取失败
```bash
# 检查网络连接
ping ghcr.io
curl -I https://ghcr.io

# 使用代理拉取（如果需要）
sudo mkdir -p /etc/systemd/system/docker.service.d
sudo tee /etc/systemd/system/docker.service.d/http-proxy.conf << EOF
[Service]
Environment="HTTP_PROXY=http://proxy.example.com:8080"
Environment="HTTPS_PROXY=http://proxy.example.com:8080"
Environment="NO_PROXY=localhost,127.0.0.1"
EOF
sudo systemctl daemon-reload
sudo systemctl restart docker

# 手动登录GitHub Container Registry（如果是私有镜像）
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

### 6.5 性能问题
```bash
# 查看容器资源使用
docker stats

# 查看磁盘IO
iostat -x 1

# 查看系统负载
top
htop

# 查看容器详细信息
docker compose exec kiroclaim ps aux
docker compose exec kiroclaim df -h
```

---

## 七、安全加固建议

### 7.1 使用反向代理（推荐）
使用Nginx或Caddy作为反向代理，提供HTTPS和域名访问：

```bash
# 安装Nginx
sudo apt install -y nginx

# 配置示例（/etc/nginx/sites-available/kiroclaim）
sudo tee /etc/nginx/sites-available/kiroclaim << 'EOF'
server {
    listen 80;
    server_name claim.yourdomain.com;
    
    # 自动跳转到HTTPS（配置SSL证书后启用）
    # return 301 https://$server_name$request_uri;
    
    location / {
        proxy_pass http://localhost:9527;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # SSE支持
        proxy_buffering off;
        proxy_cache off;
    }
}
EOF

# 启用配置
sudo ln -s /etc/nginx/sites-available/kiroclaim /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### 7.2 配置SSL证书（Let's Encrypt）
```bash
# 安装Certbot
sudo apt install -y certbot python3-certbot-nginx

# 自动配置SSL
sudo certbot --nginx -d claim.yourdomain.com

# 自动续期（已自动配置定时任务）
sudo certbot renew --dry-run
```

### 7.3 限制Docker Socket访问
```bash
# 修改docker-compose.yml，注释掉Docker socket挂载
# 如果不需要后台自动更新功能，建议禁用
cd ~/kiroclaim
nano docker-compose.yml

# 注释或删除以下行：
# - /var/run/docker.sock:/var/run/docker.sock

# 重启服务
docker compose up -d
```

### 7.4 修改默认端口
```bash
# 编辑.env文件
cd ~/kiroclaim
nano .env

# 修改PORT
PORT=8080

# 重启服务
docker compose up -d
```

### 7.5 配置防火墙
```bash
# 启用UFW
sudo ufw enable

# 仅允许必要端口
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw allow 9527/tcp  # KiroClaim（如果不用反向代理）

# 禁止直接访问MySQL
sudo ufw deny 3306/tcp

# 查看规则
sudo ufw status numbered
```

---

## 八、高级配置

### 8.1 使用外部MySQL数据库
如果已有MySQL服务器，可以不使用Docker Compose中的MySQL容器：

```bash
cd ~/kiroclaim

# 编辑docker-compose.yml，删除mysql服务
nano docker-compose.yml

# 修改.env文件
nano .env

# 配置外部MySQL连接
cat > .env << EOF
PORT=9527
DB_TYPE=mysql
DB_DSN=username:password@tcp(mysql-host:3306)/database?charset=utf8mb4&parseTime=True&loc=Local
EOF

# 启动服务
docker compose up -d
```

### 8.2 配置Prometheus监控
```bash
# 编辑.env文件，添加监控认证
cd ~/kiroclaim
nano .env

# 添加以下配置
METRICS_USER=admin
METRICS_PASS=$(openssl rand -base64 24)

# 重启服务
docker compose restart kiroclaim

# 访问监控指标（需要Basic Auth）
curl -u admin:password http://localhost:9527/metrics
```

### 8.3 自定义日志配置
```bash
# 编辑.env文件
cd ~/kiroclaim
nano .env

# 添加日志配置
LOG_FILE_ENABLED=true
LOG_FILE_PATH=logs/app.log
LOG_MAX_SIZE_MB=100
LOG_MAX_BACKUPS=10
LOG_MAX_AGE_DAYS=30
LOG_COMPRESS=true

# 重启服务
docker compose restart kiroclaim
```

### 8.4 部署多实例（负载均衡）
```bash
# 使用Nginx做负载均衡
sudo tee /etc/nginx/conf.d/upstream.conf << 'EOF'
upstream kiroclaim_backend {
    server localhost:9527;
    server localhost:9528;
    server localhost:9529;
}

server {
    listen 80;
    server_name claim.yourdomain.com;
    
    location / {
        proxy_pass http://kiroclaim_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
EOF

sudo systemctl reload nginx
```

---

## 九、常见问题FAQ

### Q1: 如何查看当前运行的版本？
```bash
docker compose exec kiroclaim ./kiroclaim --version
```

### Q2: 如何回滚到旧版本？
```bash
cd ~/kiroclaim

# 停止当前服务
docker compose down

# 拉取指定版本镜像
docker pull ghcr.io/wp13461544040/kiroclaim:v0.1.3

# 修改docker-compose.yml中的镜像标签为v0.1.3
nano docker-compose.yml

# 启动服务
docker compose up -d
```

### Q3: 数据存储在哪里？
- MySQL数据：Docker卷 `kiroclaim_mysql-data`
- 应用日志：Docker卷 `kiroclaim-logs`
- 配置文件：`~/kiroclaim/.env` 和 `~/kiroclaim/docker-compose.yml`

### Q4: 如何修改管理员密码？
在浏览器登录管理后台后，在设置页面修改。如果忘记密码，需要重新初始化数据库。

### Q5: 如何升级Go或其他依赖？
镜像已包含所有依赖，无需手动升级。直接拉取新版本镜像即可。

### Q6: 如何查看容器内部文件？
```bash
# 进入容器
docker compose exec kiroclaim sh

# 查看文件
ls -la
cat static/index.html
exit
```

### Q7: 如何清理Docker缓存？
```bash
# 清理未使用的镜像
docker image prune -a

# 清理未使用的容器
docker container prune

# 清理未使用的卷（慎用！）
docker volume prune

# 一键清理所有（非常慎用！）
docker system prune -a --volumes
```

---

## 十、快速部署脚本（一键部署）

将以下内容保存为 `deploy.sh`，然后执行 `bash deploy.sh`：

```bash
#!/bin/bash
set -e

echo "=========================================="
echo "KiroClaim Docker镜像一键部署脚本"
echo "=========================================="
echo ""

# 检查Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker未安装，请先安装Docker"
    echo "执行: curl -fsSL https://get.docker.com | sh"
    exit 1
fi

# 检查Docker Compose
if ! docker compose version &> /dev/null; then
    echo "❌ Docker Compose未安装，请先安装Docker Compose V2"
    exit 1
fi

# 创建目录
DEPLOY_DIR="$HOME/kiroclaim"
mkdir -p "$DEPLOY_DIR"
cd "$DEPLOY_DIR"
echo "✅ 部署目录: $DEPLOY_DIR"

# 下载docker-compose.yml
echo "📥 下载docker-compose.yml..."
curl -fsSL https://raw.githubusercontent.com/wp13461544040/KiroClaim/main/docker-compose.yml -o docker-compose.yml
echo "✅ docker-compose.yml下载完成"

# 生成.env文件
echo "🔐 生成环境配置文件..."
cat > .env << EOF
PORT=9527
MYSQL_DATABASE=kiroclaim
MYSQL_USER=kiroclaim
MYSQL_PASSWORD=$(openssl rand -hex 24)
MYSQL_ROOT_PASSWORD=$(openssl rand -hex 24)
EOF
echo "✅ .env文件生成完成"
echo ""
echo "⚠️  重要：请保存以下密码信息！"
echo "=========================================="
cat .env
echo "=========================================="
echo ""

# 拉取镜像
echo "📦 拉取Docker镜像..."
docker pull ghcr.io/wp13461544040/kiroclaim:latest
echo "✅ 镜像拉取完成"

# 启动服务
echo "🚀 启动服务..."
docker compose up -d
echo "✅ 服务启动完成"
echo ""

# 等待服务启动
echo "⏳ 等待服务启动（约30秒）..."
sleep 30

# 检查服务状态
echo "📊 检查服务状态..."
docker compose ps
echo ""

# 显示访问信息
SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')
echo "=========================================="
echo "✅ 部署完成！"
echo "=========================================="
echo ""
echo "📍 访问地址："
echo "   初始化页面: http://${SERVER_IP}:9527/setup"
echo "   兑换中心:   http://${SERVER_IP}:9527/redeem"
echo "   管理后台:   http://${SERVER_IP}:9527/admin"
echo ""
echo "📝 配置文件: $DEPLOY_DIR/.env"
echo "📂 部署目录: $DEPLOY_DIR"
echo ""
echo "🔍 常用命令："
echo "   查看日志: cd $DEPLOY_DIR && docker compose logs -f"
echo "   重启服务: cd $DEPLOY_DIR && docker compose restart"
echo "   停止服务: cd $DEPLOY_DIR && docker compose down"
echo "   更新服务: cd $DEPLOY_DIR && docker pull ghcr.io/wp13461544040/kiroclaim:latest && docker compose up -d"
echo ""
echo "⚠️  请记得在防火墙/安全组开放 9527 端口！"
echo "=========================================="
```

使用方法：
```bash
# 下载并执行
curl -fsSL https://raw.githubusercontent.com/wp13461544040/KiroClaim/main/deploy.sh -o deploy.sh
chmod +x deploy.sh
./deploy.sh
```

---

## 十一、技术支持

- **项目地址**：https://github.com/wp13461544040/KiroClaim
- **镜像仓库**：https://github.com/wp13461544040/KiroClaim/pkgs/container/kiroclaim
- **问题反馈**：https://github.com/wp13461544040/KiroClaim/issues

---

**文档版本**: v1.0  
**更新日期**: 2026-08-08  
**适用版本**: KiroClaim v0.1.0+
