# Fleet setup script for GameLift multi-build system
# Sets up the deployment pipeline: CloudFormation stack, S3 bucket, fleet, and CodeBuild

param(
    [Parameter(Mandatory=$true, Position=0)]
    [string]$StackName,
    
    [Parameter(Mandatory=$false, Position=1)]
    [string]$ParametersFile = ""
)

$ErrorActionPreference = "Stop"

$TemplateFile = "fleet_deployment_pipeline.yml"

Write-Host "=========================================="
Write-Host "GameLift Multi-Build Fleet Setup"
Write-Host "=========================================="
Write-Host ""
Write-Host "Stack Name:      $StackName"
Write-Host "Template:        $TemplateFile"
if ($ParametersFile) {
    Write-Host "Parameters File: $ParametersFile"
}
Write-Host ""

# Check if template exists
if (-not (Test-Path $TemplateFile)) {
    Write-Host "ERROR: Template file '$TemplateFile' not found" -ForegroundColor Red
    exit 1
}

# Prompt for ServerBinaryRelativePath if no parameters file provided
if (-not $ParametersFile) {
    Write-Host "=========================================="
    Write-Host "Configuration"
    Write-Host "=========================================="
    Write-Host ""
    Write-Host "The SERVER_BINARY_RELATIVE_PATH is the path to your game server binary"
    Write-Host "within each build version folder."
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  - gameserver                              (binary at root)"
    Write-Host "  - bin/gameserver                          (binary in subdirectory)"
    Write-Host "  - MyGame/Binaries/Linux/MyGameServer      (Unreal Engine)"
    Write-Host "  - MyGame.x86_64                           (Unity)"
    Write-Host ""
    Write-Host "See README.md for more examples and details."
    Write-Host ""
    Write-Host "Current value in wrapper.sh:" -ForegroundColor Cyan
    $currentValue = Select-String -Path "wrapper.sh" -Pattern "^SERVER_BINARY_RELATIVE_PATH=" | Select-Object -First 1
    if ($currentValue) {
        Write-Host "  $currentValue"
    } else {
        Write-Host "  (not found)"
    }
    Write-Host ""
    $binaryPath = Read-Host "Enter SERVER_BINARY_RELATIVE_PATH [press Enter to keep current]"
    
    if ($binaryPath) {
        Write-Host ""
        Write-Host "Updating wrapper.sh with: SERVER_BINARY_RELATIVE_PATH=`"$binaryPath`""
        $content = Get-Content "wrapper.sh" -Raw
        $content = $content -replace 'SERVER_BINARY_RELATIVE_PATH="[^"]*"', "SERVER_BINARY_RELATIVE_PATH=`"$binaryPath`""
        Set-Content "wrapper.sh" -Value $content -NoNewline
        Write-Host "Updated wrapper.sh" -ForegroundColor Green
    } else {
        Write-Host ""
        Write-Host "Keeping current SERVER_BINARY_RELATIVE_PATH value"
    }
    Write-Host ""
}

# Check if parameters file exists
if ($ParametersFile -and -not (Test-Path $ParametersFile)) {
    Write-Host "ERROR: Parameters file '$ParametersFile' not found" -ForegroundColor Red
    exit 1
}

# Step 1: Check if stack exists
Write-Host "=========================================="
Write-Host "Step 1: Checking CloudFormation Stack"
Write-Host "=========================================="
Write-Host ""

$stackExists = $false
try {
    $ErrorActionPreference = "SilentlyContinue"
    $describeOutput = aws cloudformation describe-stacks --stack-name $StackName 2>&1
    $ErrorActionPreference = "Stop"
    if ($LASTEXITCODE -eq 0) {
        $stackExists = $true
    }
} catch {
    $stackExists = $false
}

if (-not $stackExists) {
    Write-Host "Stack does not exist. Creating new stack..."
    Write-Host ""
    
    $ErrorActionPreference = "Continue"
    if ($ParametersFile) {
        $createOutput = aws cloudformation create-stack --stack-name $StackName --template-body file://$TemplateFile --parameters file://$ParametersFile --capabilities CAPABILITY_IAM 2>&1
    } else {
        $createOutput = aws cloudformation create-stack --stack-name $StackName --template-body file://$TemplateFile --capabilities CAPABILITY_IAM 2>&1
    }
    $createExitCode = $LASTEXITCODE
    $ErrorActionPreference = "Stop"
    
    if ($createExitCode -ne 0) {
        Write-Host ""
        Write-Host "ERROR: Failed to create stack" -ForegroundColor Red
        Write-Host ""
        Write-Host $createOutput
        Write-Host ""
        exit 1
    }
    
    Write-Host "Waiting for stack creation to complete..."
    Write-Host "(This may take 5-10 minutes)"
    Write-Host ""
    
    $ErrorActionPreference = "Continue"
    aws cloudformation wait stack-create-complete --stack-name $StackName 2>&1
    $waitExitCode = $LASTEXITCODE
    $ErrorActionPreference = "Stop"
    
    if ($waitExitCode -ne 0) {
        Write-Host ""
        Write-Host "ERROR: Stack creation failed or timed out" -ForegroundColor Red
        Write-Host ""
        Write-Host "Check stack events for details:" -ForegroundColor Yellow
        Write-Host "  aws cloudformation describe-stack-events --stack-name $StackName --max-items 20" -ForegroundColor Cyan
        Write-Host ""
        exit 1
    }
    
    Write-Host "Stack created successfully" -ForegroundColor Green
} else {
    Write-Host "Stack exists. Updating stack..."
    Write-Host ""
    
    $ErrorActionPreference = "Continue"
    if ($ParametersFile) {
        $updateOutput = aws cloudformation update-stack --stack-name $StackName --template-body file://$TemplateFile --parameters file://$ParametersFile --capabilities CAPABILITY_IAM 2>&1
    } else {
        $updateOutput = aws cloudformation update-stack --stack-name $StackName --template-body file://$TemplateFile --capabilities CAPABILITY_IAM 2>&1
    }
    $updateExitCode = $LASTEXITCODE
    $ErrorActionPreference = "Stop"
    
    if ($updateOutput -match "No updates are to be performed") {
        Write-Host "No updates needed for stack" -ForegroundColor Green
    } elseif ($updateExitCode -ne 0) {
        Write-Host "ERROR: Stack update failed" -ForegroundColor Red
        Write-Host $updateOutput
        exit 1
    } else {
        Write-Host "Waiting for stack update to complete..."
        
        $ErrorActionPreference = "Continue"
        aws cloudformation wait stack-update-complete --stack-name $StackName 2>&1
        $waitExitCode = $LASTEXITCODE
        $ErrorActionPreference = "Stop"
        
        if ($waitExitCode -ne 0) {
            Write-Host ""
            Write-Host "ERROR: Stack update failed or timed out" -ForegroundColor Red
            Write-Host ""
            Write-Host "Check stack events for details:" -ForegroundColor Yellow
            Write-Host "  aws cloudformation describe-stack-events --stack-name $StackName --max-items 20" -ForegroundColor Cyan
            Write-Host ""
            exit 1
        }
        
        Write-Host "Stack updated successfully" -ForegroundColor Green
    }
}

Write-Host ""

# Step 2: Get stack outputs
Write-Host "=========================================="
Write-Host "Step 2: Getting Stack Outputs"
Write-Host "=========================================="
Write-Host ""

$bucketName = aws cloudformation describe-stacks --stack-name $StackName --query "Stacks[0].Outputs[?OutputKey=='GameServerBuildBucketName'].OutputValue" --output text
$fleetId = aws cloudformation describe-stacks --stack-name $StackName --query "Stacks[0].Outputs[?OutputKey=='FleetId'].OutputValue" --output text

if (-not $bucketName) {
    Write-Host "ERROR: Could not get bucket name from stack outputs" -ForegroundColor Red
    exit 1
}

Write-Host "S3 Bucket:  $bucketName"
Write-Host "Fleet ID:   $fleetId"
Write-Host ""

# Step 3: Package gameserver.zip
Write-Host "=========================================="
Write-Host "Step 3: Packaging Repository"
Write-Host "=========================================="
Write-Host ""

Write-Host "Creating gameserver.zip..."
if (Test-Path "gameserver.zip") {
    Remove-Item "gameserver.zip" -Force
}

# Use .NET ZipFile to create zip with proper structure (no parent folders)
Add-Type -AssemblyName System.IO.Compression.FileSystem

$currentDir = Get-Location
$zipPath = Join-Path $currentDir "gameserver.zip"

# Create the zip file
$zip = [System.IO.Compression.ZipFile]::Open($zipPath, [System.IO.Compression.ZipArchiveMode]::Create)

try {
    # Add wrapper.sh
    Write-Host "  Adding wrapper.sh..."
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, (Join-Path $currentDir "wrapper.sh"), "wrapper.sh") | Out-Null
    
    # Add Dockerfile
    Write-Host "  Adding Dockerfile..."
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, (Join-Path $currentDir "Dockerfile"), "Dockerfile") | Out-Null
    
    # Add SdkGoWrapper directory
    Write-Host "  Adding SdkGoWrapper/..."
    $sdkFiles = Get-ChildItem -Path "SdkGoWrapper" -Recurse -File
    foreach ($file in $sdkFiles) {
        $relativePath = $file.FullName.Substring($currentDir.Path.Length + 1).Replace('\', '/')
        [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $file.FullName, $relativePath) | Out-Null
    }
    
    # Add s3-sync-sidecar directory
    Write-Host "  Adding s3-sync-sidecar/..."
    $sidecarFiles = Get-ChildItem -Path "s3-sync-sidecar" -Recurse -File
    foreach ($file in $sidecarFiles) {
        $relativePath = $file.FullName.Substring($currentDir.Path.Length + 1).Replace('\', '/')
        [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $file.FullName, $relativePath) | Out-Null
    }
    
} finally {
    $zip.Dispose()
}

Write-Host "Created gameserver.zip" -ForegroundColor Green
Write-Host ""

# Step 4: Upload to S3
Write-Host "=========================================="
Write-Host "Step 4: Uploading to S3"
Write-Host "=========================================="
Write-Host ""

aws s3 cp gameserver.zip s3://$bucketName/gameserver.zip

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to upload to S3" -ForegroundColor Red
    exit 1
}

Write-Host "Uploaded to s3://$bucketName/gameserver.zip" -ForegroundColor Green
Write-Host ""

# Step 5: Trigger CodeBuild
Write-Host "=========================================="
Write-Host "Step 5: Triggering CodeBuild"
Write-Host "=========================================="
Write-Host ""

$buildId = aws codebuild start-build --project-name GameServerBuildProject --query 'build.id' --output text

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to trigger CodeBuild" -ForegroundColor Red
    exit 1
}

Write-Host "CodeBuild started: $buildId" -ForegroundColor Green
Write-Host ""

# Summary
Write-Host "=========================================="
Write-Host "Deployment Complete!" -ForegroundColor Green
Write-Host "=========================================="
Write-Host ""
Write-Host "Fleet ID:" -ForegroundColor Cyan -NoNewline
Write-Host " $fleetId"
Write-Host "S3 Bucket:" -ForegroundColor Cyan -NoNewline
Write-Host " $bucketName"
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host ""
Write-Host "1. Monitor CodeBuild (5-10 minutes):" -ForegroundColor Green
Write-Host "   aws codebuild batch-get-builds --ids $buildId --query 'builds[0].buildStatus'" -ForegroundColor Cyan
Write-Host ""
Write-Host "2. Verify container group is READY:" -ForegroundColor Green
Write-Host "   aws gamelift describe-container-group-definition --name MyGame --query 'ContainerGroupDefinition.Status'" -ForegroundColor Cyan
Write-Host ""
Write-Host "3. Check fleet status (must be ACTIVE before creating sessions):" -ForegroundColor Green
Write-Host "   aws gamelift describe-container-fleet --fleet-id $fleetId --query 'ContainerFleet.Status'" -ForegroundColor Cyan
Write-Host ""
Write-Host "4. Upload a game build using the interactive CLI tool (recommended):" -ForegroundColor Green
Write-Host "   .\manage-builds.ps1 $StackName" -ForegroundColor Cyan
Write-Host ""
Write-Host "   Or using the upload script:"
Write-Host "   .\upload-build.ps1 .\your-build-dir v1.0.0 $bucketName" -ForegroundColor Cyan
Write-Host ""
Write-Host "5. Create a game session (after fleet is ACTIVE):" -ForegroundColor Green
Write-Host "   aws gamelift create-game-session --fleet-id $fleetId --maximum-player-session-count 2 --game-properties Key=BuildVersion,Value=v1.0.0" -ForegroundColor Cyan
Write-Host ""
