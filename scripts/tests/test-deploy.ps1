Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RootDir = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$SourceScript = Join-Path $RootDir 'scripts/deploy.ps1'
$TmpRoot = $null

function Fail([string]$Message) {
  throw "FAIL: $Message"
}

function Assert-Contains([string]$Text, [string]$Needle, [string]$Message) {
  if ($null -eq $Text -or -not $Text.Contains($Needle)) {
    Fail $Message
  }
}

function Assert-NotContains([string]$Text, [string]$Needle, [string]$Message) {
  if ($null -ne $Text -and $Text.Contains($Needle)) {
    Fail $Message
  }
}

function New-Fixture([string]$Name) {
  $fixtureRoot = Join-Path $TmpRoot $Name
  $repoRoot = Join-Path $fixtureRoot 'repo'
  $scriptsRoot = Join-Path $repoRoot 'scripts'
  $configsRoot = Join-Path $repoRoot 'configs'
  $frontendRoot = Join-Path $repoRoot 'frontend'

  New-Item -ItemType Directory -Force -Path $scriptsRoot, $configsRoot, $frontendRoot | Out-Null

  if (-not (Test-Path -LiteralPath $SourceScript)) {
    Fail "缺少 $SourceScript"
  }

  Copy-Item -LiteralPath $SourceScript -Destination (Join-Path $scriptsRoot 'deploy.ps1') -Force

  @"
services:
  app: {}
  frontend: {}
"@ | Set-Content -LiteralPath (Join-Path $repoRoot 'docker-compose.yml') -Encoding utf8

  @"
storage:
  root_dir: /data
mysql:
  dsn: test
"@ | Set-Content -LiteralPath (Join-Path $configsRoot 'config.yaml') -Encoding utf8

  @"
{
  "name": "fixture-frontend",
  "private": true,
  "scripts": {
    "test:state": "echo state",
    "test:smoke": "echo smoke",
    "build": "echo build"
  }
}
"@ | Set-Content -LiteralPath (Join-Path $frontendRoot 'package.json') -Encoding utf8

  return [pscustomobject]@{
    Root = $fixtureRoot
    Repo = $repoRoot
    Script = (Join-Path $scriptsRoot 'deploy.ps1')
    Log = Join-Path $fixtureRoot 'command.log'
    HttpCalls = Join-Path $fixtureRoot 'http-calls.log'
    PsJson = '{"Service":"app","Name":"fetch_bilibili"}' + [Environment]::NewLine + '{"Service":"frontend","Name":"fetch_bilibili_frontend"}'
    ForceHealthFail = $false
  }
}

function Reset-Mocks() {
  foreach ($name in 'docker', 'go', 'npm', 'git', 'curl', 'Invoke-WebRequest') {
    if (Test-Path "Function:$name") {
      Remove-Item "Function:$name" -Force
    }
  }
}

function Set-MockFunctions([pscustomobject]$Fixture, [switch]$FailGo, [switch]$FailNpm, [switch]$AddCurlTrap) {
  $script:CurrentFixture = $Fixture

  function global:docker {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    Add-Content -LiteralPath $script:CurrentFixture.Log -Value ("docker {0}" -f ($Args -join ' '))

    if ($Args.Count -ge 2 -and $Args[0] -eq 'compose' -and $Args[1] -eq 'ps') {
      if ($env:MOCK_PS_FAIL -eq '1') {
        throw 'mock compose ps failed'
      }
      $script:CurrentFixture.PsJson
      return
    }

    if ($Args.Count -ge 2 -and $Args[0] -eq 'compose' -and $Args[1] -eq 'restart' -and $env:MOCK_RESTART_FAIL -eq '1') {
      throw 'mock restart failed'
    }
  }

  function global:go {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    Add-Content -LiteralPath $script:CurrentFixture.Log -Value ("go {0}" -f ($Args -join ' '))
    if ($script:FailGo) {
      throw 'mock go failed'
    }
  }

  function global:npm {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    Add-Content -LiteralPath $script:CurrentFixture.Log -Value ("npm {0}" -f ($Args -join ' '))
    if ($script:FailNpm) {
      throw 'mock npm failed'
    }
  }

  function global:git {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    if ($Args -join ' ' -eq 'rev-parse --abbrev-ref HEAD') {
      'main'
      return
    }
    if ($Args -join ' ' -eq 'rev-parse --short HEAD') {
      'abcdef1'
      return
    }
    if ($Args -join ' ' -eq 'status --porcelain') {
      return
    }
  }

  if ($AddCurlTrap) {
    function global:curl {
      param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
      Add-Content -LiteralPath $script:CurrentFixture.Log -Value ("curl {0}" -f ($Args -join ' '))
      throw 'curl alias should not be used'
    }
  }

  function global:Invoke-WebRequest {
    param(
      [Parameter(Mandatory = $true)][string]$Uri,
      [string]$Method = 'Get',
      [int]$TimeoutSec = 10,
      [switch]$UseBasicParsing
    )
    Add-Content -LiteralPath $script:CurrentFixture.HttpCalls -Value ("{0} {1}" -f $Method, $Uri)
    if ($script:CurrentFixture.ForceHealthFail) {
      throw 'mock health failed'
    }

    [pscustomobject]@{
      StatusCode = 200
      Content = '{"ok":true}'
    }
  }

  $script:FailGo = [bool]$FailGo
  $script:FailNpm = [bool]$FailNpm
}

function Invoke-Deploy([pscustomobject]$Fixture, [string[]]$Args) {
  $result = [ordered]@{
    Succeeded = $true
    Output = ''
    ErrorMessage = ''
  }

  Push-Location $Fixture.Repo
  $env:MOCK_PS_FAIL = ''
  $env:MOCK_RESTART_FAIL = ''
  try {
    try {
      $result.Output = (& $Fixture.Script @Args 2>&1 | Out-String)
    } catch {
      $result.Succeeded = $false
      $result.ErrorMessage = $_.Exception.Message
      $result.Output = ($result.Output + [Environment]::NewLine + $result.ErrorMessage).Trim()
    }
  } finally {
    Pop-Location
    $env:MOCK_PS_FAIL = ''
    $env:MOCK_RESTART_FAIL = ''
  }

  [pscustomobject]$result
}

function Smoke-DefaultCommandEqualsDeployAll([pscustomobject]$Fixture) {
  Set-MockFunctions -Fixture $Fixture
  $result = Invoke-Deploy -Fixture $Fixture -Args @()
  if (-not $result.Succeeded) {
    Fail "默认命令执行失败: $($result.ErrorMessage)"
  }

  $log = (Get-Content -LiteralPath $Fixture.Log -Raw)
  Assert-Contains $result.Output '- 命令：deploy-all' '默认命令不是 deploy-all'
  Assert-Contains $log 'go test ./... -count=1' '默认 deploy-all 应执行 go test'
  Assert-Contains $log 'npm run test:state' '默认 deploy-all 应执行前端状态测试'
  Assert-Contains $log 'npm run test:smoke' '默认 deploy-all 应执行前端快速测试'
  Assert-Contains $log 'npm run build' '默认 deploy-all 应执行前端构建'
  Assert-Contains $log 'docker compose up -d --build' '默认 deploy-all 应启动 compose'
}

function Smoke-NoVerifySkipsVerifyButNotBuild([pscustomobject]$Fixture) {
  Set-MockFunctions -Fixture $Fixture
  $result = Invoke-Deploy -Fixture $Fixture -Args @('--no-verify')
  if (-not $result.Succeeded) {
    Fail "--no-verify 执行失败: $($result.ErrorMessage)"
  }

  $log = (Get-Content -LiteralPath $Fixture.Log -Raw)
  Assert-NotContains $log 'go test ./... -count=1' '--no-verify 不应执行 go test'
  Assert-NotContains $log 'npm run test:state' '--no-verify 不应执行前端状态测试'
  Assert-NotContains $log 'npm run test:smoke' '--no-verify 不应执行前端快速测试'
  Assert-Contains $log 'npm run build' '--no-verify 仍应执行前端构建'
}

function Smoke-RestartFailsWhenContainerMissing([pscustomobject]$Fixture) {
  Set-MockFunctions -Fixture $Fixture
  $Fixture.PsJson = '{"Service":"app","Name":"fetch_bilibili"}'

  $result = Invoke-Deploy -Fixture $Fixture -Args @('restart')
  if ($result.Succeeded) {
    Fail 'restart 在 frontend 容器缺失时应失败'
  }
  Assert-Contains $result.Output '请先执行 deploy-all' 'restart 缺少容器时应提示先 deploy-all'
}

function Smoke-RestartParsesJsonWithSpacingAndFieldOrder([pscustomobject]$Fixture) {
  Set-MockFunctions -Fixture $Fixture
  $Fixture.PsJson = @'
{ "Name": "fetch_bilibili", "Service": "app" }
{ "State": "running", "Service": "frontend", "Name": "fetch_bilibili_frontend" }
'@

  $result = Invoke-Deploy -Fixture $Fixture -Args @('restart')
  if (-not $result.Succeeded) {
    Fail "restart 应能解析包含空格/字段顺序变化的 JSON: $($result.ErrorMessage)"
  }

  $log = (Get-Content -LiteralPath $Fixture.Log -Raw)
  Assert-Contains $log 'docker compose restart app frontend' 'restart 识别容器存在后应继续执行重启'
}

function Smoke-StatusPrintsSummaryOnHealthFailure([pscustomobject]$Fixture) {
  Set-MockFunctions -Fixture $Fixture
  $Fixture.ForceHealthFail = $true

  $result = Invoke-Deploy -Fixture $Fixture -Args @('status')
  if (-not $result.Succeeded) {
    Fail "status 在健康检查失败时仍应返回成功: $($result.ErrorMessage)"
  }

  $log = (Get-Content -LiteralPath $Fixture.Log -Raw)
  Assert-Contains $result.Output '状态摘要' 'status 在健康检查失败时仍需输出摘要'
  Assert-Contains $log 'docker compose ps' 'status 应查询 compose 状态'
}

function Smoke-HealthcheckDoesNotDependOnCurlAlias([pscustomobject]$Fixture) {
  Set-MockFunctions -Fixture $Fixture -AddCurlTrap
  $result = Invoke-Deploy -Fixture $Fixture -Args @('deploy-app', '--no-verify')
  if (-not $result.Succeeded) {
    Fail "deploy-app --no-verify 执行失败: $($result.ErrorMessage)"
  }

  $log = if (Test-Path -LiteralPath $Fixture.Log) { Get-Content -LiteralPath $Fixture.Log -Raw } else { '' }
  $httpLog = if (Test-Path -LiteralPath $Fixture.HttpCalls) { Get-Content -LiteralPath $Fixture.HttpCalls -Raw } else { '' }

  Assert-NotContains $log 'curl ' '健康检查不应依赖 curl 别名'
  Assert-Contains $httpLog 'http://127.0.0.1:8080/healthz' '后端健康检查应调用 Invoke-WebRequest'
}

function Main() {
  $script:TmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("deploy-ps1-smoke-{0}" -f ([guid]::NewGuid().ToString('N')))
  New-Item -ItemType Directory -Force -Path $script:TmpRoot | Out-Null

  try {
    Reset-Mocks

    Smoke-DefaultCommandEqualsDeployAll (New-Fixture 'default')
    Smoke-NoVerifySkipsVerifyButNotBuild (New-Fixture 'no-verify')
    Smoke-RestartParsesJsonWithSpacingAndFieldOrder (New-Fixture 'restart-json-variants')
    Smoke-RestartFailsWhenContainerMissing (New-Fixture 'restart-missing')
    Smoke-StatusPrintsSummaryOnHealthFailure (New-Fixture 'status-health-fail')
    Smoke-HealthcheckDoesNotDependOnCurlAlias (New-Fixture 'no-curl-alias')

    Write-Host 'deploy.ps1 smoke ok'
  } finally {
    Reset-Mocks
    if ($script:TmpRoot -and (Test-Path -LiteralPath $script:TmpRoot)) {
      Remove-Item -LiteralPath $script:TmpRoot -Recurse -Force
    }
  }
}

Main
