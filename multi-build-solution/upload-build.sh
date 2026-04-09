#!/bin/bash

# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

# Helper script to upload game server builds to S3
# Usage: ./upload-build.sh <build-directory> <version> <s3-bucket> [s3-prefix]

set -e

if [ $# -lt 3 ]; then
    echo "Usage: $0 <build-directory> <version> <s3-bucket> [s3-prefix]"
    echo ""
    echo "Arguments:"
    echo "  build-directory: Local directory containing the game server build"
    echo "  version:         Build version identifier (e.g., v1.0.0, v1.1.0)"
    echo "  s3-bucket:       S3 bucket name"
    echo "  s3-prefix:       Optional S3 prefix (default: builds/)"
    echo ""
    echo "Example:"
    echo "  $0 ./game-builds/v1.0.0 v1.0.0 multibuildtest-gameserverbuildbucket-abc123"
    echo "  $0 ./game-builds/v1.0.0 v1.0.0 multibuildtest-gameserverbuildbucket-abc123 builds/"
    echo ""
    echo "Get your bucket name from CloudFormation outputs:"
    echo "  aws cloudformation describe-stacks --stack-name <stack-name> --query 'Stacks[0].Outputs[?OutputKey==\`GameServerBuildBucketName\`].OutputValue' --output text"
    exit 1
fi

BUILD_DIR="$1"
VERSION="$2"
S3_BUCKET="$3"
S3_PREFIX="${4:-builds/}"

# Ensure S3_PREFIX ends with /
if [[ ! "$S3_PREFIX" =~ /$ ]]; then
    S3_PREFIX="${S3_PREFIX}/"
fi

S3_PATH="s3://${S3_BUCKET}/${S3_PREFIX}${VERSION}/"

echo "=========================================="
echo "GameLift Build Upload"
echo "=========================================="
echo "Build Directory: $BUILD_DIR"
echo "Version:         $VERSION"
echo "S3 Destination:  $S3_PATH"
echo "=========================================="

# Validate build directory exists
if [ ! -d "$BUILD_DIR" ]; then
    echo "ERROR: Build directory '$BUILD_DIR' does not exist"
    exit 1
fi

echo ""
echo "NOTE: Make sure your wrapper.sh is configured with the correct SERVER_BINARY_RELATIVE_PATH"
echo "      that matches the binary location in this build directory."
echo ""

# Check if version already exists in S3
echo ""
echo "Checking if version already exists in S3..."
if aws s3 ls "$S3_PATH" > /dev/null 2>&1; then
    echo -e "\033[1;33mWARNING: Version $VERSION already exists in S3\033[0m"
    echo ""
    echo -e "\033[1;31mIMPORTANT: In-place updates can cause issues if game servers are currently"
    echo -e "           using this build version. The sync process updates files directly,"
    echo -e "           which may result in partially updated builds being used.\033[0m"
    echo ""
    echo -e "\033[1;32mRecommended: Use a new version number (e.g., v1.0.1 instead of v1.0.0)\033[0m"
    echo ""
    read -p "Do you want to overwrite it? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Upload cancelled"
        exit 0
    fi
fi

# Get total size for progress tracking
echo ""
echo "Calculating build size..."
# Use -k for compatibility with both macOS and Linux, then convert to bytes
TOTAL_SIZE_KB=$(du -sk "$BUILD_DIR" | cut -f1)
TOTAL_SIZE_MB=$(echo "scale=2; $TOTAL_SIZE_KB / 1024" | bc)
echo "Total size: ${TOTAL_SIZE_MB} MB"

# Upload to S3
echo ""
echo "Uploading build to S3..."
echo ""

# Use AWS CLI with performance optimizations:
# --no-progress shows file-by-file progress instead of per-file progress bars
# Parallel transfers are enabled by default in AWS CLI v2
aws s3 sync "$BUILD_DIR" "$S3_PATH" \
    --delete \
    --no-progress \
    2>&1 | while IFS= read -r line; do
        echo "$line"
        # Count uploaded files for basic progress
        if [[ "$line" =~ ^upload: ]]; then
            UPLOADED=$((UPLOADED + 1))
        fi
    done

if [ $? -eq 0 ]; then
    echo ""
    echo "Creating upload completion marker..."
    # Use ISO 8601 format compatible with both macOS and Linux
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "Upload completed at $TIMESTAMP" > /tmp/.upload_complete
    aws s3 cp /tmp/.upload_complete "${S3_PATH}.upload_complete"
    rm /tmp/.upload_complete
    
    echo ""
    echo "Updating builds marker file to trigger sync..."
    echo "Builds updated at $TIMESTAMP" > /tmp/.builds_updated
    aws s3 cp /tmp/.builds_updated "s3://${S3_BUCKET}/${S3_PREFIX}.builds_updated"
    rm /tmp/.builds_updated
    
    echo ""
    echo "=========================================="
    echo "Upload completed successfully!"
    echo "=========================================="
    echo ""
    echo "To use this build version, create a game session with:"
    echo ""
    echo "  aws gamelift create-game-session \\"
    echo "    --fleet-id <your-fleet-id> \\"
    echo "    --maximum-player-session-count 10 \\"
    echo "    --game-properties Key=BuildVersion,Value=$VERSION"
    echo ""
else
    echo ""
    echo "ERROR: Upload failed"
    exit 1
fi
