# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

# Helper script to upload game server builds to S3
# Usage: .\upload-build.ps1 <build-directory> <version> <s3-bucket> [s3-prefix]

param(
    [Parameter(Mandatory=$false, Position=0)]
    [string]$BuildDir,
    
    [Parameter(Mandatory=$false, Position=1)]
    [string]$Version,
    
    [Parameter(Mandatory=$false, Position=2)]
    [string]$S3Bucket,
    
    [Parameter(Mandatory=$false, Position=3)]
    [string]$S3Prefix = "builds/"
)

$ErrorActionPreference = "Stop"

function Show-Usage {
    Write-Host "Usage: .\upload-build.ps1 <build-directory> <version> <s3-bucket> [s3-prefix]"
    Write-Host ""
    Write-Host "Arguments:"
    Write-Host "  build-directory: Local directory containing the game server build"
    Write-Host "  version:         Build version identifier (e.g., v1.0.0, v1.1.0)"
    Write-Host "  s3-bucket:       S3 bucket name"
    Write-Host "  s3-prefix:       Optional S3 prefix (default: builds/)"
    Write-Host ""
    Write-Host "Example:"
    Write-Host "  .\upload-build.ps1 .\game-builds\v1.0.0 v1.0.0 multibuildtest-gameserverbuildbucket-abc123"
    Write-Host "  .\upload-build.ps1 .\game-builds\v1.0.0 v1.0.0 multibuildtest-gameserverbuildbucket-abc123 builds/"
    Write-Host ""
    Write-Host "Get your bucket name from CloudFormation outputs:"
    Write-Host "  aws cloudformation describe-stacks --stack-name STACKNAME --query 'Stacks[0].Outputs[?OutputKey==``GameServerBuildBucketName``].OutputValue' --output text"
    exit 1
}

if (-not $BuildDir -or -not $Version -or -not $S3Bucket) {
    Show-Usage
}

# Ensure S3_PREFIX ends with /
if (-not $S3Prefix.EndsWith("/")) {
    $S3Prefix = "$S3Prefix/"
}

$S3Path = "s3://$S3Bucket/$S3Prefix$Version/"

Write-Host "=========================================="
Write-Host "GameLift Build Upload"
Write-Host "=========================================="
Write-Host "Build Directory: $BuildDir"
Write-Host "Version:         $Version"
Write-Host "S3 Destination:  $S3Path"
Write-Host "=========================================="

# Validate build directory exists
if (-not (Test-Path $BuildDir -PathType Container)) {
    Write-Host "ERROR: Build directory '$BuildDir' does not exist" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "NOTE: Make sure your wrapper.sh is configured with the correct SERVER_BINARY_RELATIVE_PATH"
Write-Host "      that matches the binary location in this build directory."
Write-Host ""

# Check if version already exists in S3
Write-Host ""
Write-Host "Checking if version already exists in S3..."
$existsCheck = aws s3 ls $S3Path
if ($existsCheck) {
    Write-Host "WARNING: Version $Version already exists in S3" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "IMPORTANT: In-place updates can cause issues if game servers are currently" -ForegroundColor Red
    Write-Host "           using this build version. The sync process updates files directly," -ForegroundColor Red
    Write-Host "           which may result in partially updated builds being used." -ForegroundColor Red
    Write-Host ""
    Write-Host "Recommended: Use a new version number instead of overwriting" -ForegroundColor Green
    Write-Host ""
    $response = Read-Host "Do you want to overwrite it? (y/N)"
    if ($response -ne "y" -and $response -ne "Y") {
        Write-Host "Upload cancelled"
        exit 0
    }
}

# Get total size for progress tracking
Write-Host ""
Write-Host "Calculating build size..."
$fileCount = (Get-ChildItem -Path $BuildDir -Recurse -File).Count
$totalSize = (Get-ChildItem -Path $BuildDir -Recurse -File | Measure-Object -Property Length -Sum).Sum
$totalSizeMB = [math]::Round($totalSize / 1MB, 2)
Write-Host "Total files: $fileCount"
Write-Host "Total size: $totalSizeMB MB"

# Upload to S3
Write-Host ""
Write-Host "Uploading build to S3..."
Write-Host ""

# Change to build directory and sync contents to avoid nested folders
Push-Location $BuildDir
try {
    $startTime = Get-Date
    $uploadOutput = aws s3 sync . $S3Path --delete --no-progress 2>&1
    $uploadedCount = 0
    
    foreach ($line in $uploadOutput) {
        if ($line -match "^upload:") {
            $uploadedCount++
            
            # Show progress every 10 files or for large milestones
            if ($uploadedCount % 10 -eq 0 -or $uploadedCount -in @(1, 5, 25, 50, 100, 250, 500, 1000)) {
                $percentComplete = [math]::Round(($uploadedCount / $fileCount) * 100, 1)
                $elapsed = (Get-Date) - $startTime
                
                if ($elapsed.TotalSeconds -gt 0) {
                    $speedFilesPerSec = [math]::Round($uploadedCount / $elapsed.TotalSeconds, 1)
                    Write-Host "  Progress: $uploadedCount/$fileCount files ($percentComplete%) | $speedFilesPerSec files/s" -ForegroundColor Cyan
                } else {
                    Write-Host "  Progress: $uploadedCount/$fileCount files ($percentComplete%)" -ForegroundColor Cyan
                }
            }
        }
    }
    
    # Final summary
    if ($uploadedCount -gt 0) {
        $elapsed = (Get-Date) - $startTime
        Write-Host ""
        Write-Host "Upload complete: $uploadedCount files | $([math]::Round($elapsed.TotalSeconds, 1))s" -ForegroundColor Green
    }
} finally {
    Pop-Location
}

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "Creating upload completion marker..."
    $timestamp = Get-Date -Format "o"
    "Upload completed at $timestamp" | Out-File -FilePath "$env:TEMP\.upload_complete" -Encoding utf8
    aws s3 cp "$env:TEMP\.upload_complete" "$S3Path.upload_complete"
    Remove-Item "$env:TEMP\.upload_complete" -Force
    
    Write-Host ""
    Write-Host "Updating builds marker file to trigger sync..."
    "Builds updated at $timestamp" | Out-File -FilePath "$env:TEMP\.builds_updated" -Encoding utf8
    aws s3 cp "$env:TEMP\.builds_updated" "s3://$S3Bucket/$S3Prefix.builds_updated"
    Remove-Item "$env:TEMP\.builds_updated" -Force
    
    Write-Host ""
    Write-Host "=========================================="
    Write-Host "Upload completed successfully!" -ForegroundColor Green
    Write-Host "=========================================="
    Write-Host ""
    Write-Host "To use this build version, create a game session with:"
    Write-Host ""
    Write-Host "  aws gamelift create-game-session --fleet-id YOUR-FLEET-ID --maximum-player-session-count 10 --game-properties Key=BuildVersion,Value=$Version"
    Write-Host ""
} else {
    Write-Host ""
    Write-Host "ERROR: Upload failed" -ForegroundColor Red
    exit 1
}
