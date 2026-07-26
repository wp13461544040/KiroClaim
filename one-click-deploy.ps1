# KiroClaim One-Click Deploy Script
# Version: 2.0.1

$ErrorActionPreference = "Continue"

function Write-Step {
    param($Step, $Total, $Message)
    Write-Host ""
    Write-Host "[$Step/$Total] $Message" -ForegroundColor Cyan
    Write-Host ("=" * 50) -ForegroundColor Gray
}

function Write-Success {
    param($Message)
    Write-Host "  [OK] $Message" -ForegroundColor Green
}

function Write-Fail {
    param($Message)
    Write-Host "  [ERROR] $Message" -ForegroundColor Red
}

function Write-Info {
    param($Message)
    Write-Host "  [INFO] $Message" -ForegroundColor Yellow
}

Clear-Host
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  KiroClaim Deploy" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$startTime = Get-Date

# Step 1: Check Environment
Write-Step 1 6 "Check System Environment"

$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($isAdmin) {
    Write-Success "Administrator: Yes"
} else {
    Write-Info "Administrator: No (some features may be limited)"
}

# Check Go
$goInstalled = $false

try {
    $null = & go version 2>&1
    if ($LASTEXITCODE -eq 0) {
        $goVersion = & go version
        $goInstalled = $true
        Write-Success "Go: $goVersion"
    }
} catch {}

if (-not $goInstalled) {
    if (Test-Path "C:\go\bin\go.exe") {
        $env:Path = "C:\go\bin;" + $env:Path
        try {
            $null = & go version 2>&1
            if ($LASTEXITCODE -eq 0) {
                $goVersion = & go version
                $goInstalled = $true
                Write-Success "Go: $goVersion"
            }
        } catch {}
    }
}

# Step 2: Install Go if needed
if (-not $goInstalled) {
    Write-Step 2 6 "Install Go"
    
    $goVer = "1.24.0"
    $goUrl = "https://go.dev/dl/go${goVer}.windows-amd64.msi"
    $tempDir = "$env:TEMP\go-install-$(Get-Date -Format 'yyyyMMddHHmmss')"
    $installerPath = "$tempDir\go-installer.msi"
    
    try {
        New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
        
        Write-Info "Downloading Go $goVer (about 150MB)..."
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $goUrl -OutFile $installerPath -UseBasicParsing -TimeoutSec 300
        Write-Success "Download complete"
        
        Write-Info "Installing Go (1-2 minutes)..."
        $process = Start-Process msiexec.exe -Wait -ArgumentList "/i `"$installerPath`" /quiet /norestart" -PassThru -NoNewWindow
        
        if ($process.ExitCode -eq 0) {
            Write-Success "Go installed"
            
            $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
            
            Start-Sleep -Seconds 3
            if (Test-Path "C:\go\bin\go.exe") {
                $env:Path = "C:\go\bin;" + $env:Path
                $goVersion = & "C:\go\bin\go.exe" version 2>&1
                $goInstalled = $true
                Write-Success "Verified: $goVersion"
            } else {
                Write-Fail "Verification failed"
                throw "Go executable not found after installation"
            }
        } else {
            throw "Installation failed: exit code $($process.ExitCode)"
        }
        
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    } catch {
        Write-Fail "Failed to install Go: $($_.Exception.Message)"
        Write-Info "Please install manually: https://go.dev/dl/"
        Write-Host ""
        Read-Host "Press Enter to exit"
        exit 1
    }
} else {
    Write-Step 2 6 "Check Go"
    Write-Success "Go already installed, skip"
}

# Step 3: Configure Environment
Write-Step 3 6 "Configure Application"

if (-not (Test-Path "go.mod")) {
    Write-Fail "go.mod not found"
    Write-Info "Please run this script in project root directory"
    Read-Host "Press Enter to exit"
    exit 1
}
Write-Success "Project files found"

# Create .env
if (-not (Test-Path ".env")) {
    if (Test-Path ".env.example") {
        Copy-Item ".env.example" ".env"
        Write-Success "Created .env"
        
        $randomSecret = -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 32 | ForEach-Object {[char]$_})
        $envContent = Get-Content ".env" -Raw
        $envContent = $envContent -replace 'JWT_SECRET=.+', "JWT_SECRET=$randomSecret"
        Set-Content -Path ".env" -Value $envContent -NoNewline
        Write-Success "Generated JWT_SECRET"
    } else {
        Write-Fail ".env.example not found"
        Read-Host "Press Enter to exit"
        exit 1
    }
} else {
    Write-Success "Config file exists"
}

if (-not (Test-Path "logs")) {
    New-Item -ItemType Directory -Path "logs" | Out-Null
    Write-Success "Created logs directory"
}

# Step 4: Build Application
Write-Step 4 6 "Build Application"

$oldProcess = Get-Process -Name "kiroclaim" -ErrorAction SilentlyContinue
if ($oldProcess) {
    Write-Info "Stopping old process..."
    Stop-Process -Name "kiroclaim" -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

if (Test-Path "kiroclaim.exe") {
    try {
        Remove-Item "kiroclaim.exe" -Force
        Write-Success "Cleaned old file"
    } catch {
        Write-Fail "Cannot delete old file"
        Read-Host "Press Enter to exit"
        exit 1
    }
}

$env:GOPROXY = "https://goproxy.cn,direct"
$env:GO111MODULE = "on"

Write-Info "Downloading dependencies..."
$downloadOutput = & go mod download 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Fail "Failed to download dependencies"
    Write-Host $downloadOutput
    Read-Host "Press Enter to exit"
    exit 1
}
Write-Success "Dependencies downloaded"

Write-Info "Building application (1-2 minutes)..."
$buildOutput = & go build -ldflags "-s -w" -o kiroclaim.exe . 2>&1

if ($LASTEXITCODE -eq 0 -and (Test-Path "kiroclaim.exe")) {
    $size = [math]::Round((Get-Item "kiroclaim.exe").Length / 1MB, 2)
    Write-Success "Build success: kiroclaim.exe ($size MB)"
} else {
    Write-Fail "Build failed"
    Write-Host $buildOutput
    Read-Host "Press Enter to exit"
    exit 1
}

# Step 5: Configure Firewall
Write-Step 5 6 "Configure Firewall"

if ($isAdmin) {
    try {
        $ruleName = "KiroClaim-9527"
        $existingRule = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
        
        if ($existingRule) {
            Write-Success "Firewall rule exists"
        } else {
            $null = New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Protocol TCP -LocalPort 9527 -Action Allow -ErrorAction Stop
            Write-Success "Added firewall rule: port 9527"
        }
    } catch {
        Write-Info "Failed to configure firewall"
    }
} else {
    Write-Info "Skipped (requires administrator)"
}

# Step 6: Start Application
Write-Step 6 6 "Start Application"

Write-Info "Starting KiroClaim..."
try {
    $process = Start-Process -FilePath ".\kiroclaim.exe" -WindowStyle Hidden -PassThru
    Start-Sleep -Seconds 3
    
    $running = Get-Process -Name "kiroclaim" -ErrorAction SilentlyContinue
    if ($running) {
        Write-Success "Application started (PID: $($running.Id))"
    } else {
        Write-Fail "Failed to start"
        if (Test-Path "logs\app.log") {
            Write-Info "Check logs: logs\app.log"
            Get-Content "logs\app.log" -Tail 10
        }
        Read-Host "Press Enter to exit"
        exit 1
    }
} catch {
    Write-Fail "Start failed: $($_.Exception.Message)"
    Read-Host "Press Enter to exit"
    exit 1
}

# Complete
$endTime = Get-Date
$duration = $endTime - $startTime

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Deploy Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Time: $([math]::Round($duration.TotalSeconds, 1)) seconds" -ForegroundColor Gray
Write-Host ""

$hostname = $env:COMPUTERNAME
$ipAddress = $null
try {
    $ipAddress = (Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { 
        $_.InterfaceAlias -notlike "*Loopback*" -and 
        $_.IPAddress -notlike "169.254.*" 
    } | Select-Object -First 1).IPAddress
} catch {}

Write-Host "Server Info:" -ForegroundColor Yellow
Write-Host "  Hostname: $hostname" -ForegroundColor White
if ($ipAddress) {
    Write-Host "  IP: $ipAddress" -ForegroundColor White
}
Write-Host ""

Write-Host "Access URLs:" -ForegroundColor Yellow
Write-Host "  http://localhost:9527/setup" -ForegroundColor Cyan
if ($ipAddress) {
    Write-Host "  http://${ipAddress}:9527/setup" -ForegroundColor Cyan
}
Write-Host ""

Write-Host "Management:" -ForegroundColor Yellow
Write-Host "  Status: Get-Process -Name kiroclaim" -ForegroundColor White
Write-Host "  Logs:   Get-Content logs\app.log -Tail 50" -ForegroundColor White
Write-Host "  Stop:   Stop-Process -Name kiroclaim" -ForegroundColor White
Write-Host ""

Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
