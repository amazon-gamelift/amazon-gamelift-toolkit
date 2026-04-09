#!/bin/bash

# Fleet setup script for GameLift multi-build system
# Sets up the deployment pipeline: CloudFormation stack, S3 bucket, fleet, and CodeBuild
# Run this once to create the baseline infrastructure, then use it again to update.

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo "=========================================="
echo "GameLift Multi-Build Fleet Setup"
echo "=========================================="
echo ""

# Check if required parameters are provided
if [ $# -lt 1 ]; then
    echo "Usage: $0 <stack-name> [parameters-file]"
    echo ""
    echo "Arguments:"
    echo "  stack-name:       CloudFormation stack name"
    echo "  parameters-file:  Optional JSON file with stack parameters"
    echo ""
    echo "Example:"
    echo "  $0 MultiBuildTest"
    echo "  $0 MultiBuildTest parameters.json"
    echo ""
    echo "This script will:"
    echo "  1. Create or update CloudFormation stack"
    echo "  2. Package gameserver.zip"
    echo "  3. Upload to S3"
    echo "  4. Trigger CodeBuild"
    exit 1
fi

STACK_NAME="$1"
PARAMETERS_FILE="${2:-}"
TEMPLATE_FILE="fleet_deployment_pipeline.yml"

echo "Stack Name:      $STACK_NAME"
echo "Template:        $TEMPLATE_FILE"
if [ -n "$PARAMETERS_FILE" ]; then
    echo "Parameters File: $PARAMETERS_FILE"
fi
echo ""

# Check if template exists
if [ ! -f "$TEMPLATE_FILE" ]; then
    echo "ERROR: Template file '$TEMPLATE_FILE' not found"
    exit 1
fi

# Prompt for ServerBinaryRelativePath if no parameters file provided
if [ -z "$PARAMETERS_FILE" ]; then
    echo "=========================================="
    echo "Configuration"
    echo "=========================================="
    echo ""
    echo "The SERVER_BINARY_RELATIVE_PATH is the path to your game server binary"
    echo "within each build version folder."
    echo ""
    echo "Examples:"
    echo "  - gameserver                              (binary at root)"
    echo "  - bin/gameserver                          (binary in subdirectory)"
    echo "  - MyGame/Binaries/Linux/MyGameServer      (Unreal Engine)"
    echo "  - MyGame.x86_64                           (Unity)"
    echo ""
    echo "See README.md for more examples and details."
    echo ""
    echo -e "${CYAN}Current value in wrapper.sh:${NC}"
    grep "^SERVER_BINARY_RELATIVE_PATH=" wrapper.sh || echo "  (not found)"
    echo ""
    echo -e "${YELLOW}Enter SERVER_BINARY_RELATIVE_PATH [press Enter to keep current]:${NC} "
    read -p "" BINARY_PATH
    
    if [ -n "$BINARY_PATH" ]; then
        echo ""
        echo "Updating wrapper.sh with: SERVER_BINARY_RELATIVE_PATH=\"$BINARY_PATH\""
        # Update wrapper.sh
        if [[ "$OSTYPE" == "darwin"* ]]; then
            # macOS
            sed -i '' "s|^SERVER_BINARY_RELATIVE_PATH=.*|SERVER_BINARY_RELATIVE_PATH=\"$BINARY_PATH\"|" wrapper.sh
        else
            # Linux
            sed -i "s|^SERVER_BINARY_RELATIVE_PATH=.*|SERVER_BINARY_RELATIVE_PATH=\"$BINARY_PATH\"|" wrapper.sh
        fi
        echo "✓ Updated wrapper.sh"
    else
        echo ""
        echo "Keeping current SERVER_BINARY_RELATIVE_PATH value"
    fi
    echo ""
fi

# Check if parameters file exists (if provided)
if [ -n "$PARAMETERS_FILE" ] && [ ! -f "$PARAMETERS_FILE" ]; then
    echo "ERROR: Parameters file '$PARAMETERS_FILE' not found"
    exit 1
fi

# Step 1: Check if stack exists
echo "=========================================="
echo "Step 1: Checking CloudFormation Stack"
echo "=========================================="
echo ""

STACK_EXISTS=$(aws cloudformation describe-stacks --stack-name "$STACK_NAME" 2>/dev/null || echo "")

if [ -z "$STACK_EXISTS" ]; then
    echo "Stack does not exist. Creating new stack..."
    
    if [ -n "$PARAMETERS_FILE" ]; then
        aws cloudformation create-stack \
            --stack-name "$STACK_NAME" \
            --template-body file://"$TEMPLATE_FILE" \
            --parameters file://"$PARAMETERS_FILE" \
            --capabilities CAPABILITY_IAM
    else
        aws cloudformation create-stack \
            --stack-name "$STACK_NAME" \
            --template-body file://"$TEMPLATE_FILE" \
            --capabilities CAPABILITY_IAM
    fi
    
    echo "Waiting for stack creation to complete..."
    aws cloudformation wait stack-create-complete --stack-name "$STACK_NAME"
    echo "✓ Stack created successfully"
else
    echo "Stack exists. Updating stack..."
    
    if [ -n "$PARAMETERS_FILE" ]; then
        UPDATE_OUTPUT=$(aws cloudformation update-stack \
            --stack-name "$STACK_NAME" \
            --template-body file://"$TEMPLATE_FILE" \
            --parameters file://"$PARAMETERS_FILE" \
            --capabilities CAPABILITY_IAM 2>&1 || echo "NO_UPDATES")
    else
        UPDATE_OUTPUT=$(aws cloudformation update-stack \
            --stack-name "$STACK_NAME" \
            --template-body file://"$TEMPLATE_FILE" \
            --capabilities CAPABILITY_IAM 2>&1 || echo "NO_UPDATES")
    fi
    
    if echo "$UPDATE_OUTPUT" | grep -q "No updates are to be performed"; then
        echo "✓ No updates needed for stack"
    else
        echo "Waiting for stack update to complete..."
        aws cloudformation wait stack-update-complete --stack-name "$STACK_NAME"
        echo "✓ Stack updated successfully"
    fi
fi

echo ""

# Step 2: Get stack outputs
echo "=========================================="
echo "Step 2: Getting Stack Outputs"
echo "=========================================="
echo ""

BUCKET_NAME=$(aws cloudformation describe-stacks --stack-name "$STACK_NAME" \
    --query 'Stacks[0].Outputs[?OutputKey==`GameServerBuildBucketName`].OutputValue' \
    --output text)

FLEET_ID=$(aws cloudformation describe-stacks --stack-name "$STACK_NAME" \
    --query 'Stacks[0].Outputs[?OutputKey==`FleetId`].OutputValue' \
    --output text)

if [ -z "$BUCKET_NAME" ]; then
    echo "ERROR: Could not get bucket name from stack outputs"
    exit 1
fi

echo "S3 Bucket:  $BUCKET_NAME"
echo "Fleet ID:   $FLEET_ID"
echo ""

# Step 3: Package gameserver.zip
echo "=========================================="
echo "Step 3: Packaging Repository"
echo "=========================================="
echo ""

echo "Creating gameserver.zip..."
zip -r gameserver.zip \
    wrapper.sh \
    SdkGoWrapper/ \
    Dockerfile \
    s3-sync-sidecar/ \
    -x "*.git*" \
    -x "*__pycache__*" \
    -x "*.DS_Store" \
    -q

if [ $? -ne 0 ]; then
    echo "ERROR: Failed to create zip file"
    exit 1
fi

echo "✓ Created gameserver.zip"
echo ""

# Step 4: Upload to S3
echo "=========================================="
echo "Step 4: Uploading to S3"
echo "=========================================="
echo ""

aws s3 cp gameserver.zip s3://$BUCKET_NAME/gameserver.zip

if [ $? -ne 0 ]; then
    echo "ERROR: Failed to upload to S3"
    exit 1
fi

echo "✓ Uploaded to s3://$BUCKET_NAME/gameserver.zip"
echo ""

# Step 5: Trigger CodeBuild
echo "=========================================="
echo "Step 5: Triggering CodeBuild"
echo "=========================================="
echo ""

BUILD_ID=$(aws codebuild start-build \
    --project-name GameServerBuildProject \
    --query 'build.id' \
    --output text)

if [ $? -ne 0 ]; then
    echo "ERROR: Failed to trigger CodeBuild"
    exit 1
fi

echo "✓ CodeBuild started: $BUILD_ID"
echo ""

# Summary
echo "=========================================="
echo -e "${GREEN}Deployment Complete!${NC}"
echo "=========================================="
echo ""
echo -e "${CYAN}Fleet ID:${NC} $FLEET_ID"
echo -e "${CYAN}S3 Bucket:${NC} $BUCKET_NAME"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo ""
echo -e "${GREEN}1. Monitor CodeBuild (5-10 minutes):${NC}"
echo -e "   ${CYAN}aws codebuild batch-get-builds --ids $BUILD_ID --query 'builds[0].buildStatus'${NC}"
echo ""
echo -e "${GREEN}2. Verify container group is READY:${NC}"
echo -e "   ${CYAN}aws gamelift describe-container-group-definition --name MyGame --query 'ContainerGroupDefinition.Status'${NC}"
echo ""
echo -e "${GREEN}3. Check fleet status (must be ACTIVE before creating sessions):${NC}"
echo -e "   ${CYAN}aws gamelift describe-container-fleet --fleet-id $FLEET_ID --query 'ContainerFleet.Status'${NC}"
echo ""
echo -e "${GREEN}4. Upload a game build using the interactive CLI tool (recommended):${NC}"
echo -e "   ${CYAN}./manage-builds.sh $STACK_NAME${NC}"
echo ""
echo "   Or using the upload script:"
echo -e "   ${CYAN}./upload-build.sh ./your-build-dir v1.0.0 $BUCKET_NAME${NC}"
echo ""
echo -e "${GREEN}5. Create a game session (after fleet is ACTIVE):${NC}"
echo -e "   ${CYAN}aws gamelift create-game-session --fleet-id $FLEET_ID --maximum-player-session-count 2 --game-properties Key=BuildVersion,Value=v1.0.0${NC}"
echo ""
