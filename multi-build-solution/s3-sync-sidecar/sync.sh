#!/bin/bash
set -e

# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

# Configuration from environment variables
S3_BUCKET="${S3_BUCKET:-}"
S3_PREFIX="${S3_PREFIX:-builds/}"
BUILDS_DIR="/shared/builds"
SYNC_INTERVAL="${SYNC_INTERVAL:-5}"
TARGET_UID=1000
TARGET_GID=1000

# Marker file configuration
MARKER_FILE=".builds_updated"
MARKER_KEY="${S3_PREFIX}${MARKER_FILE}"
LAST_MARKER_ETAG_FILE="${BUILDS_DIR}/.last_marker_etag"

echo "=================================================="
echo "S3 Sync Sidecar Starting"
echo "=================================================="
echo "S3 Bucket: s3://${S3_BUCKET}/${S3_PREFIX}"
echo "Local builds directory: ${BUILDS_DIR}"
echo "Sync interval: ${SYNC_INTERVAL} seconds"
echo "Marker file: ${MARKER_KEY}"
echo "Running as user: $(id -u) (UID), $(id -g) (GID)"
echo "Memory limit: $(cat /sys/fs/cgroup/memory/memory.limit_in_bytes 2>/dev/null | awk '{printf "%.0f MB", $1/1024/1024}' || echo 'unknown')"
echo "=================================================="

# Validate configuration
if [ -z "$S3_BUCKET" ]; then
    echo "ERROR: S3_BUCKET environment variable must be set"
    exit 1
fi

# Check if builds directory exists
if [ ! -d "$BUILDS_DIR" ]; then
    echo "ERROR: Builds directory does not exist: $BUILDS_DIR"
    exit 1
fi

# Test write permissions
if ! touch "${BUILDS_DIR}/.write-test" 2>/dev/null; then
    echo "ERROR: Cannot write to ${BUILDS_DIR}"
    ls -ld "$BUILDS_DIR"
    exit 1
fi
rm -f "${BUILDS_DIR}/.write-test"
echo "✓ Write permissions OK"

# Function to get marker file ETag
get_marker_etag() {
    local marker_path="s3://${S3_BUCKET}/${MARKER_KEY}"
    local etag=$(aws s3api head-object --bucket "$S3_BUCKET" --key "$MARKER_KEY" --query 'ETag' --output text 2>/dev/null || echo "")
    echo "$etag"
}

# Function to check if marker has changed
marker_has_changed() {
    local current_etag=$(get_marker_etag)
    
    # If marker doesn't exist in S3, consider it unchanged (no builds yet)
    if [ -z "$current_etag" ]; then
        echo "[$(date -Iseconds)] Marker file not found in S3 (no builds uploaded yet)"
        return 1
    fi
    
    # If we don't have a stored ETag, this is first run - marker has "changed"
    if [ ! -f "$LAST_MARKER_ETAG_FILE" ]; then
        echo "$current_etag" > "$LAST_MARKER_ETAG_FILE"
        echo "[$(date -Iseconds)] First run - marker ETag: $current_etag"
        return 0
    fi
    
    local last_etag=$(cat "$LAST_MARKER_ETAG_FILE")
    
    if [ "$current_etag" != "$last_etag" ]; then
        echo "[$(date -Iseconds)] Marker changed - ETag: $last_etag -> $current_etag"
        echo "$current_etag" > "$LAST_MARKER_ETAG_FILE"
        return 0
    fi
    
    return 1
}

# Function to sync from S3
sync_from_s3() {
    local s3_path="s3://${S3_BUCKET}/${S3_PREFIX}"
    echo "[$(date -Iseconds)] Syncing builds from ${s3_path}..."
    
    # First, get list of builds in S3
    local s3_ls_output
    s3_ls_output=$(aws s3 ls "$s3_path" 2>&1)
    local s3_ls_exit=$?

    # If the S3 listing fails, abort sync entirely to avoid deleting local builds
    if [ $s3_ls_exit -ne 0 ]; then
        echo "[$(date -Iseconds)] ERROR: Failed to list S3 builds (exit code: $s3_ls_exit). Skipping sync to protect local data."
        echo "$s3_ls_output"
        return 1
    fi

    local s3_builds=$(echo "$s3_ls_output" | grep "PRE" | awk '{print $2}' | sed 's/\///' || true)
    
    # Get list of local builds
    local local_builds=$(find "$BUILDS_DIR" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; || true)
    
    # Delete local builds that don't exist in S3
    local deleted_any=false
    for local_build in $local_builds; do
        if ! echo "$s3_builds" | grep -q "^${local_build}$"; then
            echo "[$(date -Iseconds)] Deleting build not in S3: $local_build"
            rm -rf "${BUILDS_DIR}/${local_build}"
            deleted_any=true
        fi
    done
    
    # Sync build files from S3
    # Exclude local marker files from sync operations
    # Important: Use ** to match files at any depth
    # Performance optimizations:
    # --no-progress: Reduces memory overhead from progress tracking
    local sync_output
    sync_output=$(aws s3 sync "$s3_path" "$BUILDS_DIR" --delete \
        --exclude ".last-sync" \
        --exclude ".last_marker_etag" \
        --exclude "**/.download_complete" \
        --no-progress 2>&1)
    local sync_exit=$?
    
    echo "$sync_output"
    
    if [ $sync_exit -eq 0 ]; then
        # Check if any files were actually transferred or if we deleted builds
        # Look for download/upload/delete operations in output
        if echo "$sync_output" | grep -qE "(download:|upload:|delete:)" || [ "$deleted_any" = true ]; then
            echo "[$(date -Iseconds)] Changes detected - files were downloaded or deleted"
            return 0  # Files changed - need permissions
        else
            echo "[$(date -Iseconds)] No changes detected - skipping permission updates"
            return 2  # No changes - skip permissions
        fi
    else
        echo "[$(date -Iseconds)] ERROR: Sync failed (exit code: $sync_exit)"
        return 1  # Error
    fi
}

# Function to set permissions
set_permissions() {
    echo "[$(date -Iseconds)] Setting ownership and permissions..."
    
    # Create marker file
    echo "Last sync: $(date -Iseconds)" > "${BUILDS_DIR}/.last-sync"
    
    # Change ownership to target UID/GID first
    chown -R ${TARGET_UID}:${TARGET_GID} "$BUILDS_DIR"
    
    # Set directory permissions (755)
    find "$BUILDS_DIR" -type d -exec chmod 755 {} \;
    
    # Set file permissions (644 by default)
    find "$BUILDS_DIR" -type f -exec chmod 644 {} \;
    
    # Make binaries executable (755)
    # Files without extension (binaries)
    find "$BUILDS_DIR" -type f ! -name '*.*' -exec chmod 755 {} \;
    # .so files
    find "$BUILDS_DIR" -type f -name '*.so*' -exec chmod 755 {} \;
    # .sh files
    find "$BUILDS_DIR" -type f -name '*.sh' -exec chmod 755 {} \;
    
    # Check each build directory for upload completion marker and create download marker
    # Do this AFTER permissions are set so markers have correct ownership
    for build_dir in "$BUILDS_DIR"/*; do
        if [ -d "$build_dir" ]; then
            local build_name=$(basename "$build_dir")
            local upload_marker="${build_dir}/.upload_complete"
            local download_marker="${build_dir}/.download_complete"
            
            # If upload marker exists and download marker doesn't, create it
            if [ -f "$upload_marker" ] && [ ! -f "$download_marker" ]; then
                echo "[$(date -Iseconds)] Marking build $build_name as download complete"
                echo "Download completed at $(date -Iseconds)" > "$download_marker"
                chown ${TARGET_UID}:${TARGET_GID} "$download_marker"
                chmod 644 "$download_marker"
            elif [ ! -f "$upload_marker" ]; then
                echo "[$(date -Iseconds)] WARNING: Build $build_name missing upload marker (may be incomplete)"
                # Remove download marker if it exists to prevent use
                rm -f "$download_marker"
            fi
        fi
    done
    
    echo "[$(date -Iseconds)] Permissions set successfully"
    
    # List available builds
    echo "[$(date -Iseconds)] Available builds:"
    ls -la "$BUILDS_DIR" | while read line; do
        echo "  $line"
    done
}

# Initial sync
echo ""
echo "Performing initial sync (may take several minutes for large builds)..."
if sync_from_s3; then
    set_permissions
fi

# Continuous sync loop with marker file optimization
echo ""
echo "Starting continuous sync loop with marker file optimization..."
echo "Checking marker file every ${SYNC_INTERVAL} seconds, syncing only when changed"
echo ""

while true; do
    sleep "$SYNC_INTERVAL"
    
    # Check if marker has changed
    if marker_has_changed; then
        echo "[$(date -Iseconds)] Marker changed - performing full sync..."
        sync_result=0
        sync_from_s3 || sync_result=$?
        
        if [ $sync_result -eq 0 ]; then
            # Files changed - run permission updates
            set_permissions
        elif [ $sync_result -eq 2 ]; then
            # No changes detected - skip permissions to save CPU
            echo "[$(date -Iseconds)] Sync completed but no changes detected"
        fi
    else
        echo "[$(date -Iseconds)] Marker unchanged - skipping sync"
    fi
done
