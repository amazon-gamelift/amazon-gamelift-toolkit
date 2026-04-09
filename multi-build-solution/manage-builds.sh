#!/bin/bash

# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

# GameLift Multi-Build Management CLI
# Manage game server builds in S3 for GameLift dynamic loading

set -e

SCRIPT_NAME=$(basename "$0")

# Colors for output
RED='\033[1;31m'
GREEN='\033[1;32m'
YELLOW='\033[1;33m'
BLUE='\033[1;34m'
CYAN='\033[1;36m'
NC='\033[0m' # No Color

# Global variables
S3_BUCKET=""
S3_PREFIX="builds/"
STACK_NAME=""

# Print usage
usage() {
    echo "GameLift Multi-Build Management CLI"
    echo ""
    echo "Usage: $SCRIPT_NAME [stack-name]"
    echo ""
    echo "Interactive Mode (recommended):"
    echo "  $SCRIPT_NAME                    # Interactive menu"
    echo "  $SCRIPT_NAME MultiBuildTest2    # Interactive with stack name"
    echo ""
    echo "Direct Commands (legacy):"
    echo "  $SCRIPT_NAME list <bucket> [prefix]"
    echo "  $SCRIPT_NAME upload <dir> <version> <bucket> [prefix]"
    echo "  $SCRIPT_NAME delete <version> <bucket> [prefix]"
    echo "  $SCRIPT_NAME info <version> <bucket> [prefix]"
    echo ""
    exit 1
}

# Get S3 bucket from CloudFormation stack
get_bucket_from_stack() {
    local stack="$1"
    
    echo -e "${CYAN}Fetching S3 bucket from CloudFormation stack: $stack${NC}"
    
    local bucket=$(aws cloudformation describe-stacks \
        --stack-name "$stack" \
        --query 'Stacks[0].Outputs[?OutputKey==`GameServerBuildBucketName`].OutputValue' \
        --output text 2>/dev/null)
    
    if [ -z "$bucket" ] || [ "$bucket" == "None" ]; then
        echo -e "${RED}ERROR: Could not find GameServerBuildBucketName output in stack '$stack'${NC}"
        echo ""
        echo "Available stacks:"
        aws cloudformation list-stacks --stack-status-filter CREATE_COMPLETE UPDATE_COMPLETE \
            --query 'StackSummaries[].StackName' --output text | tr '\t' '\n' | grep -i gamelift || echo "  (no GameLift stacks found)"
        return 1
    fi
    
    echo -e "${GREEN}✓ Found bucket: $bucket${NC}"
    echo ""
    S3_BUCKET="$bucket"
    return 0
}

# Interactive menu
show_menu() {
    clear
    echo "=========================================="
    echo "  GameLift Multi-Build Manager"
    echo "=========================================="
    echo ""
    if [ -n "$STACK_NAME" ]; then
        echo -e "Stack:  ${CYAN}$STACK_NAME${NC}"
    fi
    echo -e "Bucket: ${CYAN}s3://${S3_BUCKET}/${S3_PREFIX}${NC}"
    echo ""
    echo "=========================================="
    echo ""
    echo "1) List all builds"
    echo "2) Upload a new build"
    echo "3) Delete a build"
    echo "4) Show build info"
    echo "5) Change stack/bucket"
    echo "6) Exit"
    echo ""
    echo -n "Select an option [1-6]: "
}

# Interactive mode
interactive_mode() {
    while true; do
        show_menu
        read -r choice
        echo ""
        
        case $choice in
            1)
                cmd_list "$S3_BUCKET" "$S3_PREFIX"
                echo ""
                read -p "Press Enter to continue..."
                ;;
            2)
                echo -e "${BLUE}Upload New Build${NC}"
                echo "=========================================="
                echo ""
                read -p "Build directory path: " build_dir
                read -p "Version identifier (e.g., v1.0.0): " version
                echo ""
                cmd_upload "$build_dir" "$version" "$S3_BUCKET" "$S3_PREFIX"
                echo ""
                read -p "Press Enter to continue..."
                ;;
            3)
                echo -e "${BLUE}Delete Build${NC}"
                echo "=========================================="
                echo ""
                # Show available builds first
                cmd_list "$S3_BUCKET" "$S3_PREFIX"
                echo ""
                read -p "Version to delete: " version
                echo ""
                cmd_delete "$version" "$S3_BUCKET" "$S3_PREFIX"
                echo ""
                read -p "Press Enter to continue..."
                ;;
            4)
                echo -e "${BLUE}Build Information${NC}"
                echo "=========================================="
                echo ""
                # Show available builds first
                cmd_list "$S3_BUCKET" "$S3_PREFIX"
                echo ""
                read -p "Version to inspect: " version
                echo ""
                cmd_info "$version" "$S3_BUCKET" "$S3_PREFIX"
                echo ""
                read -p "Press Enter to continue..."
                ;;
            5)
                echo -e "${BLUE}Change Stack/Bucket${NC}"
                echo "=========================================="
                echo ""
                read -p "CloudFormation stack name: " new_stack
                if [ -n "$new_stack" ]; then
                    if get_bucket_from_stack "$new_stack"; then
                        STACK_NAME="$new_stack"
                    fi
                fi
                read -p "Press Enter to continue..."
                ;;
            6)
                echo "Goodbye!"
                exit 0
                ;;
            *)
                echo -e "${RED}Invalid option. Please select 1-6.${NC}"
                sleep 2
                ;;
        esac
    done
}

# List builds
cmd_list() {
    local bucket="$1"
    local prefix="${2:-builds/}"
    
    # Ensure prefix ends with /
    [[ ! "$prefix" =~ /$ ]] && prefix="${prefix}/"
    
    echo "=========================================="
    echo "Available Builds"
    echo "=========================================="
    echo "Bucket: s3://${bucket}/${prefix}"
    echo ""
    
    # List all directories in the prefix
    local builds=$(aws s3 ls "s3://${bucket}/${prefix}" | grep "PRE" | awk '{print $2}' | sed 's/\///')
    
    if [ -z "$builds" ]; then
        echo "No builds found"
        return
    fi
    
    echo -e "${BLUE}Version${NC}\t\t${BLUE}Status${NC}\t\t${BLUE}Size${NC}"
    echo "----------------------------------------"
    
    local total_bytes=0
    
    for build in $builds; do
        local build_path="s3://${bucket}/${prefix}${build}/"
        local marker_path="s3://${bucket}/${prefix}${build}/.upload_complete"
        
        # Check if upload marker exists
        if aws s3 ls "$marker_path" > /dev/null 2>&1; then
            local status="${GREEN}Complete${NC}"
        else
            local status="${YELLOW}Incomplete${NC}"
        fi
        
        # Get total size
        local size=$(aws s3 ls "$build_path" --recursive --summarize | grep "Total Size" | awk '{print $3}')
        if [ -z "$size" ]; then
            size="0"
        fi
        
        total_bytes=$((total_bytes + size))
        
        # Convert bytes to human readable
        local size_hr=$(numfmt --to=iec-i --suffix=B $size 2>/dev/null || echo "${size}B")
        
        echo -e "${build}\t\t${status}\t\t${size_hr}"
    done
    
    echo "----------------------------------------"
    
    # Show total size
    local total_hr=$(numfmt --to=iec-i --suffix=B $total_bytes 2>/dev/null || echo "${total_bytes}B")
    echo -e "${BLUE}Total Storage:${NC} ${total_hr}"
    
    # Calculate total in GB for warning
    local total_gb=$(echo "scale=2; $total_bytes / 1073741824" | bc 2>/dev/null || echo "0")
    
    # Warning if approaching 54GB limit
    if (( $(echo "$total_gb > 40" | bc -l 2>/dev/null || echo 0) )); then
        echo ""
        echo -e "${RED}⚠️  WARNING: Total storage is ${total_gb}GB${NC}"
        echo -e "${RED}   GameLift instances have a 54GB storage limit.${NC}"
        echo -e "${RED}   Consider deleting old build versions.${NC}"
    elif (( $(echo "$total_gb > 30" | bc -l 2>/dev/null || echo 0) )); then
        echo ""
        echo -e "${YELLOW}⚠️  Notice: Total storage is ${total_gb}GB${NC}"
        echo -e "${YELLOW}   GameLift instances have a 54GB storage limit.${NC}"
    fi
    
    echo ""
}

# Upload build
cmd_upload() {
    local build_dir="$1"
    local version="$2"
    local bucket="$3"
    local prefix="${4:-builds/}"
    
    # Validate inputs
    if [ -z "$build_dir" ] || [ -z "$version" ] || [ -z "$bucket" ]; then
        echo -e "${RED}ERROR: Missing required arguments${NC}"
        usage
    fi
    
    if [ ! -d "$build_dir" ]; then
        echo -e "${RED}ERROR: Build directory '$build_dir' does not exist${NC}"
        exit 1
    fi
    
    # Ensure prefix ends with /
    [[ ! "$prefix" =~ /$ ]] && prefix="${prefix}/"
    
    local s3_path="s3://${bucket}/${prefix}${version}/"
    
    echo "=========================================="
    echo "GameLift Build Upload"
    echo "=========================================="
    echo "Build Directory: $build_dir"
    echo "Version:         $version"
    echo "S3 Destination:  $s3_path"
    echo "=========================================="
    echo ""
    
    # Check if version already exists
    if aws s3 ls "$s3_path" > /dev/null 2>&1; then
        echo -e "${YELLOW}WARNING: Version $version already exists in S3${NC}"
        echo ""
        echo -e "${RED}IMPORTANT: In-place updates can cause issues if game servers are currently"
        echo -e "           using this build version. The sync process updates files directly,"
        echo -e "           which may result in partially updated builds being used.${NC}"
        echo ""
        echo -e "${GREEN}Recommended: Use a new version number (e.g., v1.0.1 instead of v1.0.0)${NC}"
        echo ""
        read -p "Do you want to overwrite it? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "Upload cancelled"
            exit 0
        fi
    fi
    
    # Upload to S3
    echo ""
    echo "Uploading build to S3..."
    aws s3 sync "$build_dir" "$s3_path" --delete
    
    if [ $? -eq 0 ]; then
        echo ""
        echo "Creating upload completion marker..."
        echo "Upload completed at $(date -Iseconds)" > /tmp/.upload_complete
        aws s3 cp /tmp/.upload_complete "${s3_path}.upload_complete"
        rm /tmp/.upload_complete
        
        echo ""
        echo "Updating builds marker file to trigger sync..."
        echo "Builds updated at $(date -Iseconds)" > /tmp/.builds_updated
        aws s3 cp /tmp/.builds_updated "s3://${bucket}/${prefix}.builds_updated"
        rm /tmp/.builds_updated
        
        echo ""
        echo "=========================================="
        echo -e "${GREEN}Upload completed successfully!${NC}"
        echo "=========================================="
        echo ""
        echo "To use this build version, create a game session with:"
        echo ""
        echo "  aws gamelift create-game-session \\"
        echo "    --fleet-id <your-fleet-id> \\"
        echo "    --maximum-player-session-count 10 \\"
        echo "    --game-properties Key=BuildVersion,Value=$version"
        echo ""
    else
        echo ""
        echo -e "${RED}ERROR: Upload failed${NC}"
        exit 1
    fi
}

# Delete build
cmd_delete() {
    local version="$1"
    local bucket="$2"
    local prefix="${3:-builds/}"
    
    # Validate inputs
    if [ -z "$version" ] || [ -z "$bucket" ]; then
        echo -e "${RED}ERROR: Missing required arguments${NC}"
        usage
    fi
    
    # Ensure prefix ends with /
    [[ ! "$prefix" =~ /$ ]] && prefix="${prefix}/"
    
    local s3_path="s3://${bucket}/${prefix}${version}/"
    local marker_path="s3://${bucket}/${prefix}${version}.upload_complete"
    
    # Check if build exists
    if ! aws s3 ls "$s3_path" > /dev/null 2>&1; then
        echo -e "${RED}ERROR: Build version '$version' not found${NC}"
        exit 1
    fi
    
    echo "=========================================="
    echo "Delete Build"
    echo "=========================================="
    echo "Version: $version"
    echo "Path:    $s3_path"
    echo "=========================================="
    echo ""
    echo -e "${RED}WARNING: This will permanently delete the build from S3${NC}"
    echo -e "${RED}         Game servers using this build will fail to start${NC}"
    echo ""
    read -p "Are you sure you want to delete this build? (y/N) " -n 1 -r
    echo
    
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Delete cancelled"
        exit 0
    fi
    
    echo ""
    echo "Deleting build..."
    aws s3 rm "$s3_path" --recursive
    aws s3 rm "$marker_path" 2>/dev/null || true
    
    echo ""
    echo "Updating builds marker file to trigger sync..."
    # Use ISO 8601 format compatible with both macOS and Linux
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "Builds updated at $TIMESTAMP" > /tmp/.builds_updated
    aws s3 cp /tmp/.builds_updated "s3://${bucket}/${prefix}.builds_updated"
    rm /tmp/.builds_updated
    
    echo ""
    echo -e "${GREEN}Build deleted successfully${NC}"
}

# Show build info
cmd_info() {
    local version="$1"
    local bucket="$2"
    local prefix="${3:-builds/}"
    
    # Validate inputs
    if [ -z "$version" ] || [ -z "$bucket" ]; then
        echo -e "${RED}ERROR: Missing required arguments${NC}"
        usage
    fi
    
    # Ensure prefix ends with /
    [[ ! "$prefix" =~ /$ ]] && prefix="${prefix}/"
    
    local s3_path="s3://${bucket}/${prefix}${version}/"
    local marker_path="s3://${bucket}/${prefix}${version}/.upload_complete"
    
    # Check if build exists
    if ! aws s3 ls "$s3_path" > /dev/null 2>&1; then
        echo -e "${RED}ERROR: Build version '$version' not found${NC}"
        exit 1
    fi
    
    echo "=========================================="
    echo "Build Information"
    echo "=========================================="
    echo "Version: $version"
    echo "Path:    $s3_path"
    echo ""
    
    # Check upload marker
    if aws s3 ls "$marker_path" > /dev/null 2>&1; then
        echo -e "Status:  ${GREEN}Complete${NC}"
        local marker_content=$(aws s3 cp "$marker_path" - 2>/dev/null)
        echo "Marker:  $marker_content"
    else
        echo -e "Status:  ${YELLOW}Incomplete (missing upload marker)${NC}"
    fi
    
    echo ""
    echo "Files:"
    aws s3 ls "$s3_path" --recursive --human-readable --summarize
    echo ""
}

# Main command dispatcher
main() {
    # Check if running in interactive mode or legacy command mode
    if [ $# -eq 0 ]; then
        # No arguments - prompt for stack name
        echo "=========================================="
        echo "  GameLift Multi-Build Manager"
        echo "=========================================="
        echo ""
        read -p "CloudFormation stack name: " stack_input
        if [ -z "$stack_input" ]; then
            echo -e "${RED}ERROR: Stack name is required${NC}"
            exit 1
        fi
        if get_bucket_from_stack "$stack_input"; then
            STACK_NAME="$stack_input"
            interactive_mode
        else
            exit 1
        fi
    elif [ $# -eq 1 ] && [[ ! "$1" =~ ^(list|upload|delete|info|help|--help|-h)$ ]]; then
        # Single argument that's not a command - assume it's a stack name
        if get_bucket_from_stack "$1"; then
            STACK_NAME="$1"
            interactive_mode
        else
            exit 1
        fi
    else
        # Legacy command mode
        local command="$1"
        shift
        
        case "$command" in
            list)
                cmd_list "$@"
                ;;
            upload)
                cmd_upload "$@"
                ;;
            delete)
                cmd_delete "$@"
                ;;
            info)
                cmd_info "$@"
                ;;
            help|--help|-h)
                usage
                ;;
            *)
                echo -e "${RED}ERROR: Unknown command '$command'${NC}"
                echo ""
                usage
                ;;
        esac
    fi
}

main "$@"
