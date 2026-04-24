Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
  $PSNativeCommandUseErrorActionPreference = $false
}

# Set console output encoding to UTF-8 so docker logs display Chinese correctly on Windows.
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding  = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$script:REPO_ROOT = ''
$script:DOCKER_CMD = ''
$script:GO_CMD = ''
$script:NPM_CMD = ''
$script:NO_VERIFY = $false
$script:CURRENT_CMD = ''
$script:CLI_ARGS = @($args)
$script:STEP_NO = 0
$script:HEALTH_RETRY_ATTEMPTS = if ($env:HEALTH_RETRY_ATTEMPTS) { [int]$env:HEALTH_RETRY_ATTEMPTS } else { 30 }
$script:HEALTH_RETRY_INTERVAL = if ($env:HEALTH_RETRY_INTERVAL) { [int]$env:HEALTH_RETRY_INTERVAL } else { 1 }

function Write-Info([string]$Message) {
  Write-Host $Message
}

function Write-WarnMessage([string]$Message) {
  Write-Host "[WARN] $Message"
}

function Write-Step([string]$Message) {
  $script:STEP_NO += 1
  Write-Host ("[STEP {0}] {1}" -f $script:STEP_NO, $Message)
}

function Fail([string]$Message) {
  throw "[FAIL] $Message"
}

function Resolve-RepoRoot {
  if (-not $PSCommandPath) {
    Fail 'fail: unable to resolve script path'
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
    Fail "missing file: $(Join-Path $script:REPO_ROOT 'docker-compose.yml')"
  }

  if (-not (Test-Path -LiteralPath (Join-Path $script:REPO_ROOT 'configs/config.yaml'))) {
    Fail "missing file: $(Join-Path $script:REPO_ROOT 'configs/config.yaml')"
  }

  if (-not (Test-Path -LiteralPath (Join-Path $script:REPO_ROOT '.env'))) {
    Write-WarnMessage 'missing .env, using default compose image settings'
  }
}

function Resolve-RuntimeCommands {
  $script:DOCKER_CMD = Resolve-CommandPath -Name 'docker' -FallbackPaths @(
    '/usr/local/bin/docker',
    '/Applications/Docker.app/Contents/Resources/bin/docker',
    'C:\Program Files\Docker\Docker\resources\bin\docker.exe'
  )

  if (-not $script:DOCKER_CMD) {
    Fail 'docker command not found'
  }

  if (-not (Get-Command -Name 'Invoke-WebRequest' -ErrorAction SilentlyContinue)) {
    Fail 'Invoke-WebRequest command not found'
  }
}

function Resolve-GoCommand {
  $script:GO_CMD = Resolve-CommandPath -Name 'go' -FallbackPaths @('/usr/local/go/bin/go', 'C:\Go\bin\go.exe')
  if (-not $script:GO_CMD) {
    Fail 'go command not found'
  }
}

function Resolve-NpmCommand {
  $script:NPM_CMD = Resolve-CommandPath -Name 'npm' -FallbackPaths @('/usr/local/bin/npm', 'C:\Program Files\nodejs\npm.cmd')
  if (-not $script:NPM_CMD) {
    Fail 'npm command not found'
  }
}

function Invoke-Native {
  param(
    [Parameter(Mandatory = $true)][string]$Command,
    [Parameter(Mandatory = $true)][string[]]$Arguments
  )

  $global:LASTEXITCODE = 0
  $previousErrorAction = $ErrorActionPreference
  try {
    $ErrorActionPreference = 'Continue'
    & $Command @Arguments
  } finally {
    $ErrorActionPreference = $previousErrorAction
  }
  if (($global:LASTEXITCODE -is [int]) -and $global:LASTEXITCODE -ne 0) {
    throw "command-exit-$global:LASTEXITCODE"
  }
}

function Invoke-DockerCompose {
  param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)
  $allArgs = @('compose') + $Arguments
  Invoke-Native -Command $script:DOCKER_CMD -Arguments $allArgs
}

function Test-ShouldRetryWithoutBuildKit {
  param([string]$Output)

  if ($env:DOCKER_BUILDKIT -eq '0') {
    return $false
  }

  return $Output -match 'proxyconnect tcp: dial tcp (127\.0\.0\.1|localhost|\[::1\]):\d+'
}

function Test-ComposeRunningServices {
  param([Parameter(Mandatory = $true)][string[]]$RequiredServices)

  $global:LASTEXITCODE = 0
  $previousErrorAction = $ErrorActionPreference
  try {
    $ErrorActionPreference = 'Continue'
    $raw = (& $script:DOCKER_CMD compose ps --services --filter status=running 2>$null | Out-String)
  } finally {
    $ErrorActionPreference = $previousErrorAction
  }
  if (($global:LASTEXITCODE -is [int]) -and $global:LASTEXITCODE -ne 0) {
    return $false
  }

  $running = @(
    $raw -split "`r?`n" |
    ForEach-Object { $_.Trim() } |
    Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
  )

  foreach ($service in $RequiredServices) {
    if ($running -notcontains $service) {
      return $false
    }
  }
  return $true
}

function Get-ExpectedServicesForComposeUp {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  if ($Arguments -contains 'app') {
    return @('app', 'mysql')
  }
  return @('app', 'frontend', 'mysql')
}

function Invoke-DockerComposeBuildWithFallback {
  param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)

  try {
    Invoke-DockerCompose @Arguments
    return
  } catch {
    $exitCode = if ($global:LASTEXITCODE -is [int]) { $global:LASTEXITCODE } else { 1 }

    if (($Arguments -contains 'up') -and ($Arguments -contains '-d')) {
      $required = Get-ExpectedServicesForComposeUp -Arguments $Arguments
      if (Test-ComposeRunningServices -RequiredServices $required) {
        Write-WarnMessage "docker compose returned non-zero, but required services are running: $($required -join ', ')"
        return
      }
    }

    # Retry once with BuildKit disabled for environments where BuildKit fails
    # because of local proxy / buildx issues.
    if ($env:DOCKER_BUILDKIT -ne '0') {
      Write-WarnMessage 'docker compose failed, retrying once with BuildKit disabled (DOCKER_BUILDKIT=0)'
      $originalBuildkit = $env:DOCKER_BUILDKIT

      try {
        $env:DOCKER_BUILDKIT = '0'
        Invoke-DockerCompose @Arguments
        return
      } catch {
        if (($Arguments -contains 'up') -and ($Arguments -contains '-d')) {
          $required = Get-ExpectedServicesForComposeUp -Arguments $Arguments
          if (Test-ComposeRunningServices -RequiredServices $required) {
            Write-WarnMessage "docker compose retry returned non-zero, but required services are running: $($required -join ', ')"
            return
          }
        }
      } finally {
        if ($null -eq $originalBuildkit) {
          Remove-Item Env:DOCKER_BUILDKIT -ErrorAction SilentlyContinue
        } else {
          $env:DOCKER_BUILDKIT = $originalBuildkit
        }
      }
    }

    throw "command-exit-$exitCode"
  }
}

function Invoke-BackendVerify {
  Write-Step 'Run backend verification tests'
  Push-Location $script:REPO_ROOT
  try {
    try {
      if ($env:DEPLOY_FULL_GO_TEST -eq '1') {
        Write-WarnMessage 'DEPLOY_FULL_GO_TEST=1 detected, running full go test ./...'
        Invoke-Native -Command $script:GO_CMD -Arguments @('test', './...', '-count=1')
      } else {
        # Default to stable smoke packages so deploy is not blocked by known
        # environment-dependent suites (symlink privilege, ffmpeg fixtures, etc.).
        $packages = @(
          './cmd/server',
          './internal/api/http',
          './internal/config',
          './internal/creator',
          './internal/dashboard',
          './internal/db',
          './internal/discovery',
          './internal/jobs',
          './internal/live',
          './internal/repo/mysql',
          './internal/scheduler',
          './internal/worker'
        )
        Write-WarnMessage 'Running backend smoke test set (set DEPLOY_FULL_GO_TEST=1 for full suite)'
        Invoke-Native -Command $script:GO_CMD -Arguments (@('test') + $packages + @('-count=1'))
      }
    } catch {
      Fail 'backend verification failed'
    }
  } finally {
    Pop-Location
  }
}

function Invoke-FrontendVerify {
  Write-Step 'Run frontend quick tests'
  $frontendDir = Join-Path $script:REPO_ROOT 'frontend'

  Push-Location $frontendDir
  try {
    try {
      Invoke-Native -Command $script:NPM_CMD -Arguments @('run', 'test:state')
      Invoke-Native -Command $script:NPM_CMD -Arguments @('run', 'test:smoke')
    } catch {
      Fail 'frontend quick tests failed'
    }
  } finally {
    Pop-Location
  }
}

function Invoke-FrontendBuild {
  Write-Step 'Build frontend'
  $frontendDir = Join-Path $script:REPO_ROOT 'frontend'

  Push-Location $frontendDir
  try {
    try {
      Invoke-Native -Command $script:NPM_CMD -Arguments @('run', 'build')
    } catch {
      Fail 'fail: frontend build failed'
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
  Write-Step 'Run backend health check'
  Wait-ForHealth -Url 'http://127.0.0.1:8080/healthz' -FailureMessage 'fail: backend health check failed, run: docker compose logs app --tail=200'
}

function Test-FrontendHealth {
  Write-Step 'Run frontend health check'
  Wait-ForHealth -Url 'http://127.0.0.1:5173' -FailureMessage 'fail: frontend health check failed, run: docker compose logs frontend --tail=200'
}

function Write-Summary {
  param(
    [Parameter(Mandatory = $true)][string]$Mode,
    [bool]$IncludeFrontend = $true
  )

  Write-Info 'Summary'
  Write-Info "- Command: $Mode"

  if ($script:NO_VERIFY) {
    Write-Info '- Verify: skipped (--no-verify)'
  } else {
    Write-Info '- Verify: executed'
  }

  Write-Info '- Backend: http://localhost:8080'
  if ($IncludeFrontend) {
    Write-Info '- Frontend: http://localhost:5173'
  }
}

function Invoke-DeployAll {
  Write-Step 'Check deployment environment'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands

  if (-not $script:NO_VERIFY) {
    Resolve-GoCommand
    Resolve-NpmCommand
    Invoke-BackendVerify
    Invoke-FrontendVerify
  } else {
    Resolve-NpmCommand
    Write-Step 'Skip verification stage (--no-verify)'
  }

  Invoke-FrontendBuild

  Write-Step 'Build and start all containers'
  Push-Location $script:REPO_ROOT
  try {
    try {
      Invoke-DockerComposeBuildWithFallback -Arguments @('up', '-d', '--build')
    } catch {
      Fail 'docker compose up failed'
    }
  } finally {
    Pop-Location
  }

  Test-BackendHealth
  Test-FrontendHealth
  Write-Summary -Mode 'deploy-all' -IncludeFrontend $true
}

function Invoke-DeployApp {
  Write-Step 'Check deployment environment'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands

  if (-not $script:NO_VERIFY) {
    Resolve-GoCommand
    Invoke-BackendVerify
  } else {
    Write-Step 'Skip backend verification (--no-verify)'
  }

  Write-Step 'Build and start app container'
  Push-Location $script:REPO_ROOT
  try {
    try {
      Invoke-DockerComposeBuildWithFallback -Arguments @('up', '-d', '--build', 'app')
    } catch {
      Fail 'app container deployment failed'
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
      Fail 'fail: unable to get container status'
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
      Fail 'fail: unable to get container status'
    }

    $services = @(
      $entries |
      ForEach-Object { $_.Service } |
      Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    )

    if ($services -notcontains 'app' -or $services -notcontains 'frontend') {
      Fail 'app/frontend containers not found, run deploy-all first'
    }
  } catch {
    if ($_.Exception.Message -like '*deploy-all first*') {
      throw
    }
    Fail 'fail: unable to get container status'
  } finally {
    Pop-Location
  }
}

function Invoke-Restart {
  Write-Step 'Check deployment environment'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands
  Test-RestartContainers

  Write-Step 'Restart app and frontend containers'
  Push-Location $script:REPO_ROOT
  try {
    try {
      Invoke-DockerCompose restart app frontend
    } catch {
      Fail 'fail: container restart failed'
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
  $backendOk = 'FAIL'
  $frontendOk = 'FAIL'
  $branch = 'unknown'
  $commit = 'unknown'
  $dirty = 'unknown'
  $psOutput = ''

  Write-Step 'Check deployment environment'
  Test-RequiredRepoFiles
  Resolve-RuntimeCommands

  if (Get-Command -Name 'git' -ErrorAction SilentlyContinue) {
    $branch = Get-GitValue -Args @('rev-parse', '--abbrev-ref', 'HEAD')
    $commit = Get-GitValue -Args @('rev-parse', '--short', 'HEAD')
    $statusRaw = Get-GitValue -Args @('status', '--porcelain') -DefaultValue ''
    if ([string]::IsNullOrWhiteSpace($statusRaw)) {
      $dirty = 'clean'
    } else {
      $dirty = 'dirty'
    }
  }

  Write-Step 'Collect docker compose status'
  Push-Location $script:REPO_ROOT
  try {
    try {
      $global:LASTEXITCODE = 0
      $psOutput = (& $script:DOCKER_CMD compose ps 2>&1 | Out-String).TrimEnd()
      if (($global:LASTEXITCODE -is [int]) -and $global:LASTEXITCODE -ne 0) {
        Fail 'docker compose ps failed'
      }
    } catch {
      Fail 'docker compose ps failed'
    }
  } finally {
    Pop-Location
  }

  Write-Step 'Run health checks'
  if (Test-WebHealth -Url 'http://127.0.0.1:8080/healthz') {
    $backendOk = 'PASS'
  }
  if (Test-WebHealth -Url 'http://127.0.0.1:5173') {
    $frontendOk = 'PASS'
  }

  Write-Info 'Summary'
  Write-Info "- Git branch: $branch"
  Write-Info "- Git commit: $commit"
  Write-Info "- Workspace: $dirty"
  Write-Info "- Backend health: $backendOk"
  Write-Info "- Frontend health: $frontendOk"
  Write-Info '- Docker Compose:'
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
          Fail 'only one subcommand is allowed'
        }
        $script:CURRENT_CMD = 'deploy-all'
        continue
      }
      'deploy-app' {
        if ($script:CURRENT_CMD) {
          Fail 'only one subcommand is allowed'
        }
        $script:CURRENT_CMD = 'deploy-app'
        continue
      }
      'restart' {
        if ($script:CURRENT_CMD) {
          Fail 'only one subcommand is allowed'
        }
        $script:CURRENT_CMD = 'restart'
        continue
      }
      'status' {
        if ($script:CURRENT_CMD) {
          Fail 'only one subcommand is allowed'
        }
        $script:CURRENT_CMD = 'status'
        continue
      }
      default {
        Fail "unsupported argument or command '$arg'"
      }
    }
  }

  if (-not $script:CURRENT_CMD) {
    $script:CURRENT_CMD = 'deploy-all'
  }

  if ($script:NO_VERIFY -and $script:CURRENT_CMD -notin @('deploy-all', 'deploy-app')) {
    Fail '--no-verify is only valid for deploy-all and deploy-app'
  }
}

function Main {
  Resolve-RepoRoot
  Set-Location $script:REPO_ROOT
  Parse-Args -CliArgs $script:CLI_ARGS

  switch ($script:CURRENT_CMD) {
    'deploy-all' { Invoke-DeployAll }
    'deploy-app' { Invoke-DeployApp }
    'restart' { Invoke-Restart }
    'status' { Invoke-Status }
    default { Fail "unknown command '$script:CURRENT_CMD'" }
  }
}

Main
