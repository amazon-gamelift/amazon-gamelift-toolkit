# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

# GameLift Multi-Build Management CLI
# Manage game server builds in S3 for GameLift dynamic loading

param(
    [Parameter(Mandatory=$false, Position=0)]
    [string]$StackName = "",
    
    [Parameter(Mandatory=$false)]
    [string]$Command = "",
    
    [Parameter(Mandatory=$false, ValueFromRemainingArguments=$true)]
    [string[]]$Arguments
)

$ErrorActionPreference = "Stop"

# Global variables
$script:S3Bucket = ""
$script:S3Prefix = "builds/"
$script:StackName = $StackName

function Show-Usage {
    Write-Host "GameLift Multi-Build Management CLI"
    Write-Host ""
    Write-Host "Usage: .\manage-builds.ps1 [stack-name]"
    Write-Host ""
    Write-Host "Interactive Mode (recommended):"
    Write-Host "  .\manage-builds.ps1                    # Interactive menu"
    Write-Host "  .\manage-builds.ps1 MultiBuildTest2    # Interactive with stack name"
    Write-Host ""
    Write-Host "Direct Commands (legacy):"
    Write-Host "  .\manage-builds.ps1 -Command list -Arguments <bucket> [prefix]"
    Write-Host "  .\manage-builds.ps1 -Command upload -Arguments <dir>,<version>,<bucket> [prefix]"
    Write-Host "  .\manage-builds.ps1 -Command delete -Arguments <version>,<bucket> [prefix]"
    Write-Host "  .\manage-builds.ps1 -Command info -Arguments <version>,<bucket> [prefix]"
    Write-Host ""
    exit 1
}

function Get-BucketFromStack {
    param([string]$Stack)
    
    Write-Host "Fetching S3 bucket from CloudFormation stack: $Stack" -ForegroundColor Cyan
    
    try {
        $ErrorActionPreference = "Continue"
        $bucket = aws cloudformation describe-stacks --stack-name $Stack --query "Stacks[0].Outputs[?OutputKey=='GameServerBuildBucketName'].OutputValue" --output text 2>&1 | Where-Object { $_ -is [string] -and $_ -notmatch "^(aws|An error)" }
        $ErrorActionPreference = "Stop"
    } catch {
        $bucket = $null
    }
    
    if (-not $bucket -or $bucket -eq "None" -or $bucket -match "error") {
        Write-Host "ERROR: Could not find GameServerBuildBucketName output in stack '$Stack'" -ForegroundColor Red
        Write-Host ""
        Write-Host "Available stacks:"
        try {
            $ErrorActionPreference = "Continue"
            $stackList = aws cloudformation list-stacks --stack-status-filter CREATE_COMPLETE UPDATE_COMPLETE --query "StackSummaries[].StackName" --output text 2>&1
            $ErrorActionPreference = "Stop"
            
            if ($stackList -and $stackList -is [string]) {
                $stacks = $stackList -split "`t"
                $stacks | ForEach-Object { Write-Host "  $_" }
            } else {
                Write-Host "  (could not list stacks)"
            }
        } catch {
            Write-Host "  (could not list stacks)"
        }
        return $false
    }
    
    Write-Host "Found bucket: $bucket" -ForegroundColor Green
    Write-Host ""
    $script:S3Bucket = $bucket
    return $true
}

function Show-Menu {
    Clear-Host
    Write-Host "=========================================="
    Write-Host "  GameLift Multi-Build Manager"
    Write-Host "=========================================="
    Write-Host ""
    if ($script:StackName) {
        Write-Host "Stack:  " -NoNewline
        Write-Host $script:StackName -ForegroundColor Cyan
    }
    Write-Host "Bucket: " -NoNewline
    Write-Host "s3://$($script:S3Bucket)/$($script:S3Prefix)" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "=========================================="
    Write-Host ""
    Write-Host "1) List all builds"
    Write-Host "2) Upload a new build"
    Write-Host "3) Delete a build"
    Write-Host "4) Show build info"
    Write-Host "5) Change stack/bucket"
    Write-Host "6) Exit"
    Write-Host ""
    Write-Host -NoNewline "Select an option [1-6]: "
}

function Invoke-ListBuilds {
    param(
        [string]$Bucket,
        [string]$Prefix = "builds/"
    )
    
    if (-not $Prefix.EndsWith("/")) {
        $Prefix = "$Prefix/"
    }
    
    Write-Host "=========================================="
    Write-Host "Available Builds"
    Write-Host "=========================================="
    Write-Host "Bucket: s3://$Bucket/$Prefix"
    Write-Host ""
    
    $lsOutput = aws s3 ls "s3://$Bucket/$Prefix" 2>&1
    if (-not $lsOutput) {
        Write-Host "No builds found"
        return
    }
    
    $builds = $lsOutput | Where-Object { $_ -match "PRE" } | ForEach-Object {
        if ($_ -match "PRE\s+(.+)/") {
            $matches[1]
        }
    }
    
    if (-not $builds) {
        Write-Host "No builds found"
        return
    }
    
    Write-Host "Version`t`t`tStatus`t`t`tSize" -ForegroundColor Blue
    Write-Host "----------------------------------------"
    
    $totalBytes = 0
    
    foreach ($build in $builds) {
        $buildPath = "s3://$Bucket/$Prefix$build/"
        $markerPath = "s3://$Bucket/$Prefix$build/.upload_complete"
        
        $markerExists = aws s3 ls $markerPath
        if ($markerExists) {
            $status = "Complete"
            $statusColor = "Green"
        } else {
            $status = "Incomplete"
            $statusColor = "Yellow"
        }
        
        $sizeOutput = aws s3 ls $buildPath --recursive --summarize 2>&1 | Select-String "Total Size:"
        if ($sizeOutput) {
            $size = [int64]($sizeOutput -replace ".*Total Size:\s*", "")
        } else {
            $size = 0
        }
        
        $totalBytes += $size
        
        $sizeHr = Format-FileSize $size
        
        Write-Host "$build`t`t" -NoNewline
        Write-Host $status -ForegroundColor $statusColor -NoNewline
        Write-Host "`t`t$sizeHr"
    }
    
    Write-Host "----------------------------------------"
    
    $totalHr = Format-FileSize $totalBytes
    Write-Host "Total Storage: " -ForegroundColor Blue -NoNewline
    Write-Host $totalHr
    
    $totalGb = [math]::Round($totalBytes / 1GB, 2)
    
    if ($totalGb -gt 40) {
        Write-Host ""
        Write-Host "WARNING: Total storage is ${totalGb}GB" -ForegroundColor Red
        Write-Host "   GameLift instances have a 54GB storage limit." -ForegroundColor Red
        Write-Host "   Consider deleting old build versions." -ForegroundColor Red
    } elseif ($totalGb -gt 30) {
        Write-Host ""
        Write-Host "Notice: Total storage is ${totalGb}GB" -ForegroundColor Yellow
        Write-Host "   GameLift instances have a 54GB storage limit." -ForegroundColor Yellow
    }
    
    Write-Host ""
}

function Format-FileSize {
    param([int64]$Size)
    
    if ($Size -ge 1GB) {
        return "{0:N2}GB" -f ($Size / 1GB)
    } elseif ($Size -ge 1MB) {
        return "{0:N2}MB" -f ($Size / 1MB)
    } elseif ($Size -ge 1KB) {
        return "{0:N2}KB" -f ($Size / 1KB)
    } else {
        return "${Size}B"
    }
}

function Invoke-UploadBuild {
    param(
        [string]$BuildDir,
        [string]$Version,
        [string]$Bucket,
        [string]$Prefix = "builds/"
    )
    
    if (-not $BuildDir -or -not $Version -or -not $Bucket) {
        Write-Host "ERROR: Missing required arguments" -ForegroundColor Red
        Show-Usage
    }
    
    if (-not (Test-Path $BuildDir -PathType Container)) {
        Write-Host "ERROR: Build directory '$BuildDir' does not exist" -ForegroundColor Red
        exit 1
    }
    
    if (-not $Prefix.EndsWith("/")) {
        $Prefix = "$Prefix/"
    }
    
    $s3Path = "s3://$Bucket/$Prefix$Version/"
    
    Write-Host "=========================================="
    Write-Host "GameLift Build Upload"
    Write-Host "=========================================="
    Write-Host "Build Directory: $BuildDir"
    Write-Host "Version:         $Version"
    Write-Host "S3 Destination:  $s3Path"
    Write-Host "=========================================="
    Write-Host ""
    
    $existsCheck = aws s3 ls $s3Path 2>&1
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
            return
        }
    }
    
    Write-Host ""
    Write-Host "Calculating build size..."
    $fileCount = (Get-ChildItem -Path $BuildDir -Recurse -File).Count
    $totalSize = (Get-ChildItem -Path $BuildDir -Recurse -File | Measure-Object -Property Length -Sum).Sum
    $totalSizeMB = [math]::Round($totalSize / 1MB, 2)
    Write-Host "Total files: $fileCount"
    Write-Host "Total size: $totalSizeMB MB"
    
    Write-Host ""
    Write-Host "Uploading build to S3..."
    Write-Host ""
    
    # Change to build directory and sync contents to avoid nested folders
    Push-Location $BuildDir
    try {
        $startTime = Get-Date
        $uploadOutput = aws s3 sync . $s3Path --delete --no-progress 2>&1
        $uploadedCount = 0
        $uploadedBytes = 0
    
        foreach ($line in $uploadOutput) {
            if ($line -match "^upload:") {
                $uploadedCount++
                
                # Try to extract file size from the line (if available)
                if ($line -match "upload:.*\s+(\d+)\s+bytes") {
                    $uploadedBytes += [int64]$matches[1]
                }
                
                # Show progress every 10 files or for large milestones
                if ($uploadedCount % 10 -eq 0 -or $uploadedCount -in @(1, 5, 25, 50, 100, 250, 500, 1000)) {
                    $percentComplete = [math]::Round(($uploadedCount / $fileCount) * 100, 1)
                    $elapsed = (Get-Date) - $startTime
                    $uploadedMB = [math]::Round($uploadedBytes / 1MB, 2)
                    
                    if ($elapsed.TotalSeconds -gt 0) {
                        $speedMBps = [math]::Round($uploadedMB / $elapsed.TotalSeconds, 2)
                        Write-Host "  Progress: $uploadedCount/$fileCount files ($percentComplete%) | $uploadedMB MB uploaded | $speedMBps MB/s" -ForegroundColor Cyan
                    } else {
                        Write-Host "  Progress: $uploadedCount/$fileCount files ($percentComplete%)" -ForegroundColor Cyan
                    }
                }
            } elseif ($line -match "delete:" -or $line -match "Completed") {
                Write-Host $line -ForegroundColor DarkGray
            }
        }
        
        # Final summary
        if ($uploadedCount -gt 0) {
            $elapsed = (Get-Date) - $startTime
            $uploadedMB = [math]::Round($uploadedBytes / 1MB, 2)
            Write-Host ""
            Write-Host "Upload complete: $uploadedCount files | $uploadedMB MB | $([math]::Round($elapsed.TotalSeconds, 1))s" -ForegroundColor Green
        }
    } finally {
        Pop-Location
    }
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Host "Creating upload completion marker..."
        $timestamp = Get-Date -Format "o"
        "Upload completed at $timestamp" | Out-File -FilePath "$env:TEMP\.upload_complete" -Encoding utf8
        aws s3 cp "$env:TEMP\.upload_complete" "$s3Path.upload_complete"
        Remove-Item "$env:TEMP\.upload_complete" -Force
        
        Write-Host ""
        Write-Host "Updating builds marker file to trigger sync..."
        "Builds updated at $timestamp" | Out-File -FilePath "$env:TEMP\.builds_updated" -Encoding utf8
        aws s3 cp "$env:TEMP\.builds_updated" "s3://$Bucket/$Prefix.builds_updated"
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
}

function Invoke-DeleteBuild {
    param(
        [string]$Version,
        [string]$Bucket,
        [string]$Prefix = "builds/"
    )
    
    if (-not $Version -or -not $Bucket) {
        Write-Host "ERROR: Missing required arguments" -ForegroundColor Red
        Show-Usage
    }
    
    if (-not $Prefix.EndsWith("/")) {
        $Prefix = "$Prefix/"
    }
    
    $s3Path = "s3://$Bucket/$Prefix$Version/"
    $markerPath = "s3://$Bucket/$Prefix$Version.upload_complete"
    
    $existsCheck = aws s3 ls $s3Path
    if (-not $existsCheck) {
        Write-Host "ERROR: Build version '$Version' not found" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "=========================================="
    Write-Host "Delete Build"
    Write-Host "=========================================="
    Write-Host "Version: $Version"
    Write-Host "Path:    $s3Path"
    Write-Host "=========================================="
    Write-Host ""
    Write-Host "WARNING: This will permanently delete the build from S3" -ForegroundColor Red
    Write-Host "         Game servers using this build will fail to start" -ForegroundColor Red
    Write-Host ""
    $response = Read-Host "Are you sure you want to delete this build? (y/N)"
    
    if ($response -ne "y" -and $response -ne "Y") {
        Write-Host "Delete cancelled"
        return
    }
    
    Write-Host ""
    Write-Host "Deleting build..."
    aws s3 rm $s3Path --recursive
    aws s3 rm $markerPath
    
    Write-Host ""
    Write-Host "Updating builds marker file to trigger sync..."
    $timestamp = Get-Date -Format "o"
    "Builds updated at $timestamp" | Out-File -FilePath "$env:TEMP\.builds_updated" -Encoding utf8
    aws s3 cp "$env:TEMP\.builds_updated" "s3://$Bucket/$Prefix.builds_updated"
    Remove-Item "$env:TEMP\.builds_updated" -Force
    
    Write-Host ""
    Write-Host "Build deleted successfully" -ForegroundColor Green
}

function Invoke-ShowBuildInfo {
    param(
        [string]$Version,
        [string]$Bucket,
        [string]$Prefix = "builds/"
    )
    
    if (-not $Version -or -not $Bucket) {
        Write-Host "ERROR: Missing required arguments" -ForegroundColor Red
        Show-Usage
    }
    
    if (-not $Prefix.EndsWith("/")) {
        $Prefix = "$Prefix/"
    }
    
    $s3Path = "s3://$Bucket/$Prefix$Version/"
    $markerPath = "s3://$Bucket/$Prefix$Version/.upload_complete"
    
    $existsCheck = aws s3 ls $s3Path
    if (-not $existsCheck) {
        Write-Host "ERROR: Build version '$Version' not found" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "=========================================="
    Write-Host "Build Information"
    Write-Host "=========================================="
    Write-Host "Version: $Version"
    Write-Host "Path:    $s3Path"
    Write-Host ""
    
    $markerExists = aws s3 ls $markerPath
    if ($markerExists) {
        Write-Host "Status:  " -NoNewline
        Write-Host "Complete" -ForegroundColor Green
        $markerContent = aws s3 cp $markerPath - 2>&1
        Write-Host "Marker:  $markerContent"
    } else {
        Write-Host "Status:  " -NoNewline
        Write-Host "Incomplete (missing upload marker)" -ForegroundColor Yellow
    }
    
    Write-Host ""
    Write-Host "Files:"
    aws s3 ls $s3Path --recursive --human-readable --summarize
    Write-Host ""
}

function Start-InteractiveMode {
    while ($true) {
        Show-Menu
        $choice = Read-Host
        Write-Host ""
        
        switch ($choice) {
            "1" {
                Invoke-ListBuilds -Bucket $script:S3Bucket -Prefix $script:S3Prefix
                Write-Host ""
                Read-Host "Press Enter to continue"
            }
            "2" {
                Write-Host "Upload New Build" -ForegroundColor Blue
                Write-Host "=========================================="
                Write-Host ""
                $buildDir = Read-Host "Build directory path"
                $version = Read-Host "Version identifier"
                Write-Host ""
                Invoke-UploadBuild -BuildDir $buildDir -Version $version -Bucket $script:S3Bucket -Prefix $script:S3Prefix
                Write-Host ""
                Read-Host "Press Enter to continue"
            }
            "3" {
                Write-Host "Delete Build" -ForegroundColor Blue
                Write-Host "=========================================="
                Write-Host ""
                Invoke-ListBuilds -Bucket $script:S3Bucket -Prefix $script:S3Prefix
                Write-Host ""
                $version = Read-Host "Version to delete"
                Write-Host ""
                Invoke-DeleteBuild -Version $version -Bucket $script:S3Bucket -Prefix $script:S3Prefix
                Write-Host ""
                Read-Host "Press Enter to continue"
            }
            "4" {
                Write-Host "Build Information" -ForegroundColor Blue
                Write-Host "=========================================="
                Write-Host ""
                Invoke-ListBuilds -Bucket $script:S3Bucket -Prefix $script:S3Prefix
                Write-Host ""
                $version = Read-Host "Version to inspect"
                Write-Host ""
                Invoke-ShowBuildInfo -Version $version -Bucket $script:S3Bucket -Prefix $script:S3Prefix
                Write-Host ""
                Read-Host "Press Enter to continue"
            }
            "5" {
                Write-Host "Change Stack/Bucket" -ForegroundColor Blue
                Write-Host "=========================================="
                Write-Host ""
                $newStack = Read-Host "CloudFormation stack name"
                if ($newStack) {
                    if (Get-BucketFromStack -Stack $newStack) {
                        $script:StackName = $newStack
                    }
                }
                Read-Host "Press Enter to continue"
            }
            "6" {
                Write-Host "Goodbye!"
                exit 0
            }
            default {
                Write-Host "Invalid option. Please select 1-6." -ForegroundColor Red
                Start-Sleep -Seconds 2
            }
        }
    }
}

# Main
if (-not $Command) {
    if (-not $StackName) {
        Write-Host "=========================================="
        Write-Host "  GameLift Multi-Build Manager"
        Write-Host "=========================================="
        Write-Host ""
        $stackInput = Read-Host "CloudFormation stack name"
        if (-not $stackInput) {
            Write-Host "ERROR: Stack name is required" -ForegroundColor Red
            exit 1
        }
        $StackName = $stackInput
    }
    
    if (Get-BucketFromStack -Stack $StackName) {
        $script:StackName = $StackName
        Start-InteractiveMode
    } else {
        exit 1
    }
} else {
    switch ($Command.ToLower()) {
        "list" {
            Invoke-ListBuilds -Bucket $Arguments[0] -Prefix $Arguments[1]
        }
        "upload" {
            Invoke-UploadBuild -BuildDir $Arguments[0] -Version $Arguments[1] -Bucket $Arguments[2] -Prefix $Arguments[3]
        }
        "delete" {
            Invoke-DeleteBuild -Version $Arguments[0] -Bucket $Arguments[1] -Prefix $Arguments[2]
        }
        "info" {
            Invoke-ShowBuildInfo -Version $Arguments[0] -Bucket $Arguments[1] -Prefix $Arguments[2]
        }
        default {
            Write-Host "ERROR: Unknown command '$Command'" -ForegroundColor Red
            Write-Host ""
            Show-Usage
        }
    }
}
