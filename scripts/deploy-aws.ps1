<#
.SYNOPSIS
  Builds the Victory API image and deploys it to AWS App Runner.

.DESCRIPTION
  1. Pushes the Docker image to ECR (repository "victory-api").
  2. Stores secrets from the env file in AWS Secrets Manager (victory/<NAME>);
     plain settings are passed as environment variables.
  3. Creates the IAM roles App Runner needs (once).
  4. Creates the App Runner service, or deploys the new image to it.
  Run again to deploy a new version. Requires the AWS CLI (aws configure) and
  Docker Desktop running.

.EXAMPLE
  ./scripts/deploy-aws.ps1 -Region eu-central-1
#>
param(
  [string]$Region = "eu-central-1",
  [string]$Service = "victory-api",
  [string]$EnvFile = ".env.production",
  [string]$Cpu = "0.25 vCPU",
  [string]$Memory = "0.5 GB"
)
$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)

function Aws { & aws.exe --region $Region @args; if ($LASTEXITCODE) { throw "aws $($args[0]) $($args[1]) failed" } }
# Runs aws.exe for its exit code only: "not found" answers are expected and
# must not stop the script (Windows PowerShell treats them as errors).
function Quiet { $ErrorActionPreference = "Continue"; & aws.exe @args 2>$null | Out-Null }
function AwsJson { (Aws @args --output json) | Out-String | ConvertFrom-Json }
function WriteJson($obj) {
  $path = [IO.Path]::GetTempFileName()
  [IO.File]::WriteAllText($path, ($obj | ConvertTo-Json -Depth 20), (New-Object Text.UTF8Encoding $false))
  "file://$path"
}

# Settings that are secrets go to Secrets Manager; the rest are plain env vars.
$SecretNames = @("DATABASE_URL", "JWT_SECRET", "CLOUDINARY_URL", "TELEGRAM_LOGIN_BOT_TOKEN",
  "SMTP_PASS", "CHAPA_SECRET_KEY", "CHAPA_WEBHOOK_SECRET", "ANTHROPIC_API_KEY", "BOOTSTRAP_ADMIN_PASSWORD")
$Skip = @("PORT", "UPLOAD_DIR")

if (-not (Test-Path $EnvFile)) { throw "Missing $EnvFile (copy .env.example and fill in production values)." }
$vars = [ordered]@{}
foreach ($line in Get-Content $EnvFile) {
  if ($line -match '^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$' -and $Matches[2] -ne "") {
    $vars[$Matches[1]] = $Matches[2].Trim('"')
  }
}
$vars["APP_ENV"] = "production"
foreach ($required in "DATABASE_URL", "JWT_SECRET", "CORS_ORIGINS") {
  if (-not $vars.Contains($required)) { throw "$required must be set in $EnvFile." }
}

$account = (AwsJson sts get-caller-identity).Account
Write-Host "AWS account $account, region $Region"

# 1. Image -> ECR ------------------------------------------------------------
$repo = "victory-api"
$registry = "$account.dkr.ecr.$Region.amazonaws.com"
Quiet --region $Region ecr describe-repositories --repository-names $repo
if ($LASTEXITCODE) {
  Aws ecr create-repository --repository-name $repo --image-scanning-configuration scanOnPush=true | Out-Null
  Write-Host "Created ECR repository $repo"
}
$tag = (git rev-parse --short HEAD 2>$null)
if (-not $tag) { $tag = Get-Date -Format "yyyyMMddHHmm" }
$tag = "$tag-$(Get-Date -Format 'HHmmss')"
$image = "$registry/${repo}:$tag"
docker build -t $image . ; if ($LASTEXITCODE) { throw "docker build failed" }
(Aws ecr get-login-password) | docker login --username AWS --password-stdin $registry | Out-Null
docker push $image ; if ($LASTEXITCODE) { throw "docker push failed" }
Write-Host "Pushed $image"

# 2. Secrets -------------------------------------------------------------------
$secretRefs = @{}
foreach ($name in $SecretNames) {
  if (-not $vars.Contains($name)) { continue }
  $id = "victory/$name"
  $ErrorActionPreference = "Continue"
  $existing = & aws.exe --region $Region secretsmanager describe-secret --secret-id $id --output json 2>$null
  $ErrorActionPreference = "Stop"
  if ($LASTEXITCODE) {
    $arn = (AwsJson secretsmanager create-secret --name $id --secret-string $vars[$name]).ARN
  } else {
    Aws secretsmanager put-secret-value --secret-id $id --secret-string $vars[$name] | Out-Null
    $arn = ($existing | Out-String | ConvertFrom-Json).ARN
  }
  $secretRefs[$name] = $arn
  $vars.Remove($name)
}
Write-Host "Stored $($secretRefs.Count) secrets in Secrets Manager (victory/*)"

# 3. IAM roles (once) --------------------------------------------------------
function EnsureRole($name, $principal) {
  Quiet iam get-role --role-name $name
  if ($LASTEXITCODE) {
    $trust = @{ Version = "2012-10-17"; Statement = @(@{ Effect = "Allow"; Principal = @{ Service = $principal }; Action = "sts:AssumeRole" }) }
    & aws.exe iam create-role --role-name $name --assume-role-policy-document (WriteJson $trust) | Out-Null
    if ($LASTEXITCODE) { throw "Could not create role $name" }
    Write-Host "Created role $name"
    Start-Sleep -Seconds 10 # let IAM propagate
  }
  "arn:aws:iam::${account}:role/$name"
}
$accessRole = EnsureRole "victory-apprunner-ecr" "build.apprunner.amazonaws.com"
& aws.exe iam attach-role-policy --role-name victory-apprunner-ecr --policy-arn arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess | Out-Null
$instanceRole = EnsureRole "victory-apprunner-instance" "tasks.apprunner.amazonaws.com"
$readSecrets = @{ Version = "2012-10-17"; Statement = @(@{ Effect = "Allow"; Action = "secretsmanager:GetSecretValue"; Resource = "arn:aws:secretsmanager:${Region}:${account}:secret:victory/*" }) }
& aws.exe iam put-role-policy --role-name victory-apprunner-instance --policy-name read-victory-secrets --policy-document (WriteJson $readSecrets) | Out-Null
if ($LASTEXITCODE) { throw "Could not grant the instance role access to secrets" }

# 4. App Runner service ----------------------------------------------------------
function SourceConfig($envVars) {
  @{
    ImageRepository = @{
      ImageIdentifier = $image
      ImageRepositoryType = "ECR"
      ImageConfiguration = @{
        Port = "8080"
        RuntimeEnvironmentVariables = $envVars
        RuntimeEnvironmentSecrets = $secretRefs
      }
    }
    AuthenticationConfiguration = @{ AccessRoleArn = $accessRole }
    AutoDeploymentsEnabled = $false
  }
}
$existing = (AwsJson apprunner list-services).ServiceSummaryList | Where-Object { $_.ServiceName -eq $Service }
if ($existing) {
  # Keep the public address set by the first deploy.
  if (-not $vars.Contains("PUBLIC_BASE_URL")) { $vars["PUBLIC_BASE_URL"] = "https://$($existing.ServiceUrl)" }
  Aws apprunner update-service --service-arn $existing.ServiceArn --source-configuration (WriteJson (SourceConfig $vars)) | Out-Null
  $arn = $existing.ServiceArn
  Write-Host "Deploying the new image to $Service…"
} else {
  $create = @{
    ServiceName = $Service
    SourceConfiguration = SourceConfig $vars
    InstanceConfiguration = @{ Cpu = $Cpu; Memory = $Memory; InstanceRoleArn = $instanceRole }
    HealthCheckConfiguration = @{ Protocol = "HTTP"; Path = "/healthz"; Interval = 10; Timeout = 5; HealthyThreshold = 1; UnhealthyThreshold = 5 }
  }
  $arn = (AwsJson apprunner create-service --cli-input-json (WriteJson $create)).Service.ServiceArn
  Write-Host "Creating $Service (this takes a few minutes)…"
}

do {
  Start-Sleep -Seconds 15
  $svc = (AwsJson apprunner describe-service --service-arn $arn).Service
  Write-Host "  status: $($svc.Status)"
} while ($svc.Status -eq "OPERATION_IN_PROGRESS")
if ($svc.Status -ne "RUNNING") { throw "Service is $($svc.Status). See the App Runner console logs." }

$url = "https://$($svc.ServiceUrl)"
# First deploy: tell the API its own public address (used in links it creates).
if (-not $vars.Contains("PUBLIC_BASE_URL")) {
  $vars["PUBLIC_BASE_URL"] = $url
  Aws apprunner update-service --service-arn $arn --source-configuration (WriteJson (SourceConfig $vars)) | Out-Null
  do {
    Start-Sleep -Seconds 15
    $svc = (AwsJson apprunner describe-service --service-arn $arn).Service
  } while ($svc.Status -eq "OPERATION_IN_PROGRESS")
}
Write-Host ""
Write-Host "Victory API is live: $url"
Write-Host "Health: $url/healthz"
