@echo off
chcp 65001 >nul
title KiroClaim 一键部署
color 0B

:: 检查管理员权限
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo ========================================
    echo   需要管理员权限
    echo ========================================
    echo.
    echo 请右键此文件，选择"以管理员身份运行"
    echo.
    pause
    exit /b 1
)

echo ========================================
echo   KiroClaim 一键部署脚本
echo ========================================
echo.
echo 此脚本将自动完成:
echo   [1] 检查并安装 Go 环境
echo   [2] 下载项目依赖
echo   [3] 编译应用程序
echo   [4] 配置防火墙
echo   [5] 生成安全配置
echo   [6] 启动应用服务
echo.
echo 按任意键开始部署，或按 Ctrl+C 取消...
pause >nul

powershell.exe -ExecutionPolicy Bypass -File "%~dp0one-click-deploy.ps1"

if %errorlevel% equ 0 (
    echo.
    echo ========================================
    echo   部署完成！
    echo ========================================
    echo.
    echo 访问地址: http://localhost:9527/setup
    echo.
) else (
    echo.
    echo ========================================
    echo   部署失败，请查看上方错误信息
    echo ========================================
    echo.
)

pause
