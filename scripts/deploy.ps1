Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:REPO_ROOT = ''
$script:DOCKER_CMD = ''
$script:GO_CMD = ''
$script:NPM_CMD = ''
$script:NO_VERIFY = $false
$script:CURRENT_CMD = ''
$script:STEP_NO = 0
$script:HEALTH_RETRY_ATTEMPTS = if ($env:HEALTH_RETRY_ATTEMPTS) { [int]$env:HEALTH_RETRY_ATTEMPTS } else { 30 }
$script:HEALTH_RETRY_INTERVAL = if ($env:HEALTH_RETRY_INTERVAL) { [int]$env:HEALTH_RETRY_INTERVAL } else { 1 }

function Write-Info([string]$Message) {
  Write-Host $Message
}

function Write-WarnMessage([string]$Message) {
  Write-Host "[警告] $Message"
}

function Write-Step([string]$Message) {
  $script:STEP_NO += 1
  Write-Host ("[步骤 {0}] {1}" -f $script:STEP_NO, $Message)
}

function Fail([string]$Message) {
  throw "[失败] $Message"
}

function Resolve-RepoRoot {
  if (-not $PSCommandPath) {
    Fail '失败原因：无法解析脚本路径'
  }

  $scriptDir = Split-Path -Parent $PSCommandPath
  $script:REPO_ROOT = (Resolve-Path (Join-Path $scriptDir '..')).Path
}

function Resolve-CommandPath {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [string[]]$FallbackPaths = @()
  )

  $cmd = Get-Command -Name $Name -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($cmd) {
    if ($cmd.Path) {
      return $cmd.Path
    }
    return $cmd.Name
  }

  foreach ($path in $FallbackPaths) {
    if (Test-Path -LiteralPath $path) {
      return (Resolve-Path -LiteralPath $path).Path
    }
  }

  return $null
}

function Test-RequiredRepoFiles {
  if (-not (Test-Path -LiteralPath (Join-Path $script:REPO_ROOT 'docker-compose.yml'))) {
    Fail "失败原因：未找到 $(Join-Path $script:REPO_ROOT 'docker-compose.yml')"
  }

  if (-not (Test-Path -LiteralPath (Join-Path $script:REPO_ROOT 'configs/config.yaml'))) {
    Fail "失败原因：未找到 $(Join-Path $script:REPO_ROOT 'configs/config.yaml')"
  }

  if (-not (Test-Path -LiteralPath (Join-Path $script:REPO_ROOT '.env'))) {
    Write-WarnMessage '未找到 .env，将使用 Compose 默认镜像配置'
  }
}

function Resolve-RuntimeCommands {
  $script:DOCKER_CMD = Resolve-CommandPath -Name 'docker' -FallbackPaths @(
    '/usr/local/bin/docker',
    '/Applications/Docker.app/Contents/Resources/bin/docker',
    'C:\Program Files\Docker\Docker\resources\bin\docker.exe'
  )

  if (-not $script:DOCKER_CMD) {
    Fail '失败原因：未找到 docker 命令'
  }

  if (-not (Get-Command -Name 'Invoke-WebRequest' -ErrorAction SilentlyContinue)) {
    Fail '失败原因：未找到 Invoke-WebRequest 命令'
  }
}

function Resolve-GoCommand {
  $script:GO_CMD = Resolve-CommandPath -Name 'go' -FallbackPaths @('/usr/local/go/bin/go', 'C:\Go\bin\go.exe')
  if (-not $script:GO_CMD) {
    Fail '失败原因：未找到 go 命令'
  }
}

function Resolve-NpmCommand {
  $script:NPM_CMD = Resolve-CommandPath -Name 'npm' -FallbackPaths @('/usr/local/bin/npm', 'C:\Program Files\nodejs\npm.cmd')
  if (-not $script:NPM_CMD) {
    Fail '失败原因：未找到 npm 命令'
  }
}

function Invoke-Native {
  param(
    [Parameter(Mandatory = $true)][string]$Command,
    [Parameter(Mandatory = $true)][string[]]$Arguments
  )

  $global:LASTEXITCODE = 0
  & $Command @Arguments
  if (($global:LASTEXITCODE -is [int]) -and $global:LASTEXITCODE -ne 0) {
    throw "command-exit-$global:LASTEXITCODE"
  }
}

function Invoke-DockerCompose {
  param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)
  $allArgs = @('compose') + $Arguments
  Invoke-Native -Command $script:DOCKER_CMD -Arguments $allArgs
}

function Invoke-BackendVerify {
  Write-Step '执行后端测试'
  Push-Location $script:REPO_ROOT
  try {
    try {
      Invoke-Native -Command $script:GO_CMD -Arguments @('test', './...', '-count=1')
    } catch {
      Fail '失败原因：后端测试未通过'
    }
  } finally {
    Pop-Location
  }
}

function Invoke-FrontendVerify {
  Write-Step '执行前端快速测试'
  $frontendDir = Join-Path $script:REPO_ROOT 'frontend'

  Push-Location $frontendDir
  try {
    try {
      Invoke-Native -Command $script:NPM_CMD -Arguments @('run', 'test:state')
      Invoke-Native -Command $script:NPM_CMD -Arguments @('run', 'test:smoke')
    } catch {
      Fail '失败原因：前端快速测试未通过'
    }
  } finally {
    Pop-Location
  }
}

function Invoke-FrontendBuild {
  Write-Step '执行前端构建'
  $frontendDir = Join-Path $script:REPO_ROOT 'frontend'

  Push-Location $frontendDir
  try {
    try {
      Invoke-Native -Command $script:NPM_CMD -Arguments @('run', 'build')
    } catch {
      Fail '失败原因：前端构建失败'
    }
  } finally {
    Pop-Location
  }
}

function Test-WebHealth {
  param([Parameter(Mandatory = $true)][string]$Url)

  try {
    Invoke-WebRequest -Uri $Url -Method Get -TimeoutSec 5 -UseBasicParsing | Out-Null
    return $true
  } catch {
    return $false
  }
}

function Wait-ForHealth {
  param(
    [Parameter(Mandatory = $true)][string]$Url,
    [Parameter(Mandatory = $true)][string]$FailureMessage
  )

  for ($attempt = 1; $attempt -le $script:HEALTH_RETRY_ATTEMPTS; $attempt += 1) {
    if (Test-WebHealth -Url $Url) {
      return
    }

    if ($attempt -eq $script:HEALTH_RETRY_ATTEMPTS) {
      Fail $FailureMessage
    }

    Start-Sleep -Seconds $script:HEALTH_RETRY_INTERVAL
  }

  Fail $FailureMessage
}

function Test-BackendHealth {
  Write-Step '执行后端健康检查'
  Wait-ForHealth -Url 'http://127.0.0.1:8080/healthz' -FailureMessage '失败原因：后端健康检查失败，请执行 docker compose logs app --tail=200 排查'
}

function Test-FrontendHealth {
  Write-Step '执行前端健康检查'
  Wait-ForHealth -Url 'http://127.0.0.1:5173' -FailureMessage '失败原因：前端健康检查失败，请执行 docker compose logs frontend --tail=200 排查'
}

function Write-Summary {
  param(
    [Parameter(Mandatory = $true)][string]$Mode,
    [bool]$IncludeFrontend = $true
  )

  Write-Info '状态摘要'
  Write-Info "- 命令：$Mode"

  if ($script:NO_VERIFY) {
    Write-Info '- 校验：已跳过 (--no-verify)'
  } else {
    Write-Info '- 校验：已执行'
  }

  Write-Info '- 后端地址：http://localhost:8080'
  if ($IncludeFrontend) {
    Write-Info '- 前端地址：http://localhost:5173'
  }
}

function Invoke-DeployAll {
  Write-Step '检查部署环境'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands

  if (-not $script:NO_VERIFY) {
    Resolve-GoCommand
    Resolve-NpmCommand
    Invoke-BackendVerify
    Invoke-FrontendVerify
  } else {
    Resolve-NpmCommand
    Write-Step '跳过验证阶段 (--no-verify)'
  }

  Invoke-FrontendBuild

  Write-Step '构建并启动全部容器'
  Push-Location $script:REPO_ROOT
  try {
    try {
      Invoke-DockerCompose up -d --build
    } catch {
      Fail '失败原因：docker compose up 执行失败'
    }
  } finally {
    Pop-Location
  }

  Test-BackendHealth
  Test-FrontendHealth
  Write-Summary -Mode 'deploy-all' -IncludeFrontend $true
}

function Invoke-DeployApp {
  Write-Step '检查部署环境'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands

  if (-not $script:NO_VERIFY) {
    Resolve-GoCommand
    Invoke-BackendVerify
  } else {
    Write-Step '跳过后端验证 (--no-verify)'
  }

  Write-Step '构建并启动 app 容器'
  Push-Location $script:REPO_ROOT
  try {
    try {
      Invoke-DockerCompose up -d --build app
    } catch {
      Fail '失败原因：app 容器部署失败'
    }
  } finally {
    Pop-Location
  }

  Test-BackendHealth
  Write-Summary -Mode 'deploy-app' -IncludeFrontend $false
}

function Test-RestartContainers {
  Push-Location $script:REPO_ROOT
  try {
    $global:LASTEXITCODE = 0
    $psOutput = (& $script:DOCKER_CMD compose ps --format json 2>&1 | Out-String)
    if (($global:LASTEXITCODE -is [int]) -and $global:LASTEXITCODE -ne 0) {
      Fail '失败原因：无法获取容器状态'
    }

    $entries = @()
    $trimmed = $psOutput.Trim()

    try {
      if (-not [string]::IsNullOrWhiteSpace($trimmed)) {
        if ($trimmed.StartsWith('[')) {
          $parsed = $trimmed | ConvertFrom-Json -ErrorAction Stop
          if ($parsed -is [System.Collections.IEnumerable] -and -not ($parsed -is [string])) {
            foreach ($item in $parsed) {
              $entries += ,$item
            }
          } else {
            $entries += ,$parsed
          }
        } else {
          foreach ($line in ($psOutput -split "`r?`n")) {
            $lineTrim = $line.Trim()
            if ([string]::IsNullOrWhiteSpace($lineTrim)) {
              continue
            }
            $entries += ,($lineTrim | ConvertFrom-Json -ErrorAction Stop)
          }
        }
      }
    } catch {
      Fail '失败原因：无法获取容器状态'
    }

    $services = @(
      $entries |
      ForEach-Object { $_.Service } |
      Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    )

    if ($services -notcontains 'app' -or $services -notcontains 'frontend') {
      Fail '失败原因：未找到 app/frontend 容器，请先执行 deploy-all'
    }
  } catch {
    if ($_.Exception.Message -like '*请先执行 deploy-all*') {
      throw
    }
    Fail '失败原因：无法获取容器状态'
  } finally {
    Pop-Location
  }
}

function Invoke-Restart {
  Write-Step '检查部署环境'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands
  Test-RestartContainers

  Write-Step '重启 app 与 frontend 容器'
  Push-Location $script:REPO_ROOT
  try {
    try {
      Invoke-DockerCompose restart app frontend
    } catch {
      Fail '失败原因：容器重启失败'
    }
  } finally {
    Pop-Location
  }

  Test-BackendHealth
  Test-FrontendHealth
  Write-Summary -Mode 'restart' -IncludeFrontend $true
}

function Get-GitValue {
  param(
    [Parameter(Mandatory = $true)][string[]]$Args,
    [string]$DefaultValue = 'unknown'
  )

  try {
    Push-Location $script:REPO_ROOT
    try {
      $global:LASTEXITCODE = 0
      $result = (& git @Args 2>$null | Out-String).Trim()
      if (($global:LASTEXITCODE -is [int]) -and $global:LASTEXITCODE -ne 0) {
        return $DefaultValue
      }
      if ([string]::IsNullOrWhiteSpace($result)) {
        return $DefaultValue
      }
      return $result
    } finally {
      Pop-Location
    }
  } catch {
    return $DefaultValue
  }
}

function Invoke-Status {
  $backendOk = '失败'
  $frontendOk = '失败'
  $branch = 'unknown'
  $commit = 'unknown'
  $dirty = 'unknown'
  $psOutput = ''

  Write-Step '检查部署环境'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands

  if (Get-Command -Name 'git' -ErrorAction SilentlyContinue) {
    $branch = Get-GitValue -Args @('rev-parse', '--abbrev-ref', 'HEAD')
    $commit = Get-GitValue -Args @('rev-parse', '--short', 'HEAD')
    $statusRaw = Get-GitValue -Args @('status', '--porcelain') -DefaultValue ''
    if ([string]::IsNullOrWhiteSpace($statusRaw)) {
      $dirty = '干净'
    } else {
      $dirty = '有改动'
    }
  }

  Write-Step '收集 Docker Compose 状态'
  Push-Location $script:REPO_ROOT
  try {
    try {
      $global:LASTEXITCODE = 0
      $psOutput = (& $script:DOCKER_CMD compose ps 2>&1 | Out-String).TrimEnd()
      if (($global:LASTEXITCODE -is [int]) -and $global:LASTEXITCODE -ne 0) {
        Fail '失败原因：docker compose ps 执行失败'
      }
    } catch {
      Fail '失败原因：docker compose ps 执行失败'
    }
  } finally {
    Pop-Location
  }

  Write-Step '执行健康检查'
  if (Test-WebHealth -Url 'http://127.0.0.1:8080/healthz') {
    $backendOk = '通过'
  }
  if (Test-WebHealth -Url 'http://127.0.0.1:5173') {
    $frontendOk = '通过'
  }

  Write-Info '状态摘要'
  Write-Info "- Git 分支：$branch"
  Write-Info "- Git 提交：$commit"
  Write-Info "- 工作区：$dirty"
  Write-Info "- 后端健康检查：$backendOk"
  Write-Info "- 前端健康检查：$frontendOk"
  Write-Info '- Docker Compose 状态：'
  Write-Info $psOutput
}

function Parse-Args {
  param([string[]]$CliArgs)

  foreach ($arg in $CliArgs) {
    switch ($arg) {
      '--no-verify' {
        $script:NO_VERIFY = $true
        continue
      }
      'deploy-all' {
        if ($script:CURRENT_CMD) {
          Fail '失败原因：只能指定一个子命令'
        }
        $script:CURRENT_CMD = 'deploy-all'
        continue
      }
      'deploy-app' {
        if ($script:CURRENT_CMD) {
          Fail '失败原因：只能指定一个子命令'
        }
        $script:CURRENT_CMD = 'deploy-app'
        continue
      }
      'restart' {
        if ($script:CURRENT_CMD) {
          Fail '失败原因：只能指定一个子命令'
        }
        $script:CURRENT_CMD = 'restart'
        continue
      }
      'status' {
        if ($script:CURRENT_CMD) {
          Fail '失败原因：只能指定一个子命令'
        }
        $script:CURRENT_CMD = 'status'
        continue
      }
      default {
        Fail "失败原因：不支持的参数或命令 '$arg'"
      }
    }
  }

  if (-not $script:CURRENT_CMD) {
    $script:CURRENT_CMD = 'deploy-all'
  }

  if ($script:NO_VERIFY -and $script:CURRENT_CMD -notin @('deploy-all', 'deploy-app')) {
    Fail '失败原因：--no-verify 仅适用于 deploy-all 与 deploy-app'
  }
}

function Main {
  Resolve-RepoRoot
  Set-Location $script:REPO_ROOT
  Parse-Args -CliArgs $args

  switch ($script:CURRENT_CMD) {
    'deploy-all' { Invoke-DeployAll }
    'deploy-app' { Invoke-DeployApp }
    'restart' { Invoke-Restart }
    'status' { Invoke-Status }
    default { Fail "失败原因：未知命令 '$script:CURRENT_CMD'" }
  }
}

Main
