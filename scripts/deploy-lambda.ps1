<#
.SYNOPSIS
  Deploys the Victory API to AWS Lambda (container image + public function URL).

.DESCRIPTION
  Builds Dockerfile.lambda, pushes it to ECR, creates/updates the function
  "victory-api" with the settings in the env file (Lambda keeps environment
  variables encrypted at rest), and opens a public HTTPS function URL.
  Run again to deploy a new version. Needs the AWS CLI and Docker Desktop.

.EXAMPLE
  ./scripts/deploy-lambda.ps1 -Region eu-north-1
#>
param(
  [string]$Region = "eu-north-1",
  [string]$Function = "victory-api",
  [string]$EnvFile = ".env.production",
  [int]$MemoryMb = 1024,
  [int]$TimeoutSec = 60
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

if (-not (Test-Path $EnvFile)) { throw "Missing $EnvFile." }
$vars = [ordered]@{}
foreach ($line in Get-Content $EnvFile) {
  if ($line -match '^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$' -and $Matches[2] -ne "") {
    $vars[$Matches[1]] = $Matches[2].Trim('"')
  }
}
$vars["APP_ENV"] = "production"
foreach ($skip in "PORT", "UPLOAD_DIR") { $vars.Remove($skip) }
foreach ($required in "DATABASE_URL", "JWT_SECRET", "CORS_ORIGINS") {
  if (-not $vars.Contains($required)) { throw "$required must be set in $EnvFile." }
}

$account = (AwsJson sts get-caller-identity).Account
Write-Host "AWS account $account, region $Region"

# 1. Image -> ECR --------------------------------------------------------------
$repo = "victory-api"
$registry = "$account.dkr.ecr.$Region.amazonaws.com"
Quiet --region $Region ecr describe-repositories --repository-names $repo
if ($LASTEXITCODE) {
  Aws ecr create-repository --repository-name $repo --image-scanning-configuration scanOnPush=true | Out-Null
  Write-Host "Created ECR repository $repo"
}
$tag = "$(git rev-parse --short HEAD 2>$null)-$(Get-Date -Format 'yyyyMMddHHmmss')".TrimStart("-")
$image = "$registry/${repo}:$tag"
# Lambda needs a plain image manifest (no provenance attestation).
docker build --provenance=false --platform linux/amd64 -f Dockerfile.lambda -t $image . ; if ($LASTEXITCODE) { throw "docker build failed" }
(Aws ecr get-login-password) | docker login --username AWS --password-stdin $registry | Out-Null
docker push $image ; if ($LASTEXITCODE) { throw "docker push failed" }
Write-Host "Pushed $image"

# 2. Execution role (once) -------------------------------------------------------
$roleName = "victory-lambda-exec"
Quiet iam get-role --role-name $roleName
if ($LASTEXITCODE) {
  $trust = @{ Version = "2012-10-17"; Statement = @(@{ Effect = "Allow"; Principal = @{ Service = "lambda.amazonaws.com" }; Action = "sts:AssumeRole" }) }
  & aws.exe iam create-role --role-name $roleName --assume-role-policy-document (WriteJson $trust) | Out-Null
  if ($LASTEXITCODE) { throw "Could not create role $roleName" }
  & aws.exe iam attach-role-policy --role-name $roleName --policy-arn arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole | Out-Null
  Write-Host "Created role $roleName; waiting for IAM to propagate…"
  Start-Sleep -Seconds 15
}
$roleArn = "arn:aws:iam::${account}:role/$roleName"

# 3. Function -------------------------------------------------------------------
Quiet --region $Region lambda get-function --function-name $Function
$exists = -not $LASTEXITCODE
if ($exists) {
  Aws lambda update-function-code --function-name $Function --image-uri $image | Out-Null
  Aws lambda wait function-updated-v2 --function-name $Function
  Write-Host "Updated $Function to the new image"
} else {
  Aws lambda create-function --function-name $Function --package-type Image `
    --code ImageUri=$image --role $roleArn --architectures x86_64 `
    --memory-size $MemoryMb --timeout $TimeoutSec | Out-Null
  Aws lambda wait function-active-v2 --function-name $Function
  Write-Host "Created $Function"
}

# 4. Public HTTPS URL ------------------------------------------------------------
Quiet --region $Region lambda get-function-url-config --function-name $Function
if ($LASTEXITCODE) {
  # CORS is handled by the API itself, so none is configured on the URL.
  Aws lambda create-function-url-config --function-name $Function --auth-type NONE | Out-Null
  Aws lambda add-permission --function-name $Function --statement-id public-url `
    --action lambda:InvokeFunctionUrl --principal "*" --function-url-auth-type NONE | Out-Null
  # Newer accounts also need InvokeFunction for calls through the URL.
  Quiet --region $Region lambda add-permission --function-name $Function --statement-id public-url-invoke `
    --action lambda:InvokeFunction --principal "*" --invoked-via-function-url
}
$url = (AwsJson lambda get-function-url-config --function-name $Function).FunctionUrl.TrimEnd("/")

# 5. Settings (the API needs to know its own address) -------------------------
if (-not $vars.Contains("PUBLIC_BASE_URL")) { $vars["PUBLIC_BASE_URL"] = $url }
Aws lambda update-function-configuration --function-name $Function `
  --memory-size $MemoryMb --timeout $TimeoutSec `
  --environment (WriteJson @{ Variables = $vars }) | Out-Null
Aws lambda wait function-updated-v2 --function-name $Function

Write-Host "Checking $url/healthz …"
for ($i = 0; $i -lt 10; $i++) {
  try {
    $health = Invoke-RestMethod "$url/healthz" -TimeoutSec 30
    if ($health.status -eq "ok") { break }
  } catch { Start-Sleep -Seconds 5 }
}
Write-Host ""
Write-Host "Victory API is live: $url"
Write-Host "Health: $(($health | ConvertTo-Json -Compress))"
