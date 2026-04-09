# Sample Build Structure

This directory shows the expected structure for game server builds that will be uploaded to S3.

## Directory Structure

Each build version should be organized as follows:

```
v1.0.0/                    # Version identifier (can be any string)
├── gameserver             # Main executable (MUST BE REFERENCED CORRECTLY IN wrapper.sh)
├── assets etc.            # Any other assets that come with your game server build
```

## Requirements

1. **Consistent Binary Path**: Configure `SERVER_BINARY_RELATIVE_PATH` in `wrapper.sh` to match your binary location
2. **Same Structure Across Versions**: All build versions must use the same relative path
3. **Permissions**: The binary must be executable (chmod +x)
4. **Self-Contained**: Include all dependencies needed to run the server
5. **Linux Binary**: Must be a Linux x86_64 binary

## Example Builds

### Minimal Build
```
v1.0.0/
└── gameserver             # Just the executable
```
Configure: `SERVER_BINARY_RELATIVE_PATH="gameserver"`

### Unreal Engine Build
```
v1.0.0/
├── MyGame/
│   ├── Content/
│   ├── Config/
│   └── Binaries/
│       └── Linux/
│           └── MyGameServer
└── Engine/
    └── Binaries/
```
Configure: `SERVER_BINARY_RELATIVE_PATH="MyGame/Binaries/Linux/MyGameServer"`

### Unity Build
```
v1.0.0/
├── MyGame.x86_64
├── MyGame_Data/
│   ├── Managed/
│   ├── Resources/
│   └── StreamingAssets/
└── UnityPlayer.so
```
Configure: `SERVER_BINARY_RELATIVE_PATH="MyGame.x86_64"`

### Build with Subdirectory
```
v1.0.0/
├── bin/
│   └── gameserver
├── config/
└── assets/
```
Configure: `SERVER_BINARY_RELATIVE_PATH="bin/gameserver"`

## Uploading to S3

**IMPORTANT:** Always use the provided upload scripts to ensure proper marker files are created. These marker files are critical for:
- Build integrity verification (`.upload_complete`)
- Triggering sync on GameLift instances (`.builds_updated`)

### Using the Upload Script (Recommended)

**Linux/macOS:**
```bash
# From the repository root
./upload-build.sh ./game-builds/v1.0.0 v1.0.0 YOUR-BUCKET-NAME
```

**Windows (PowerShell):**
```powershell
# From the repository root
.\upload-build.ps1 .\game-builds\v1.0.0 v1.0.0 YOUR-BUCKET-NAME
```

The script automatically:
- Uploads all files to S3
- Creates `.upload_complete` marker for build integrity
- Updates `.builds_updated` marker to trigger sync on instances
- Shows progress and completion status

### Using the Interactive CLI Tool (Easiest)

**Linux/macOS:**
```bash
./manage-builds.sh MyStackName
```

**Windows (PowerShell):**
```powershell
.\manage-builds.ps1 MyStackName
```

Select "Upload a new build" from the menu and follow the prompts.

### Manual Upload (Not Recommended)

If you must use AWS CLI directly, you need to create the marker files manually:

```bash
# Upload the build
aws s3 sync ./game-builds/v1.0.0 s3://my-bucket/builds/v1.0.0/ --delete

# Create upload completion marker
echo "Upload completed at $(date -Iseconds)" > /tmp/.upload_complete
aws s3 cp /tmp/.upload_complete s3://my-bucket/builds/v1.0.0/.upload_complete

# Update builds marker to trigger sync
echo "Builds updated at $(date -Iseconds)" > /tmp/.builds_updated
aws s3 cp /tmp/.builds_updated s3://my-bucket/builds/.builds_updated
```

**Warning:** Missing marker files will cause builds to be rejected or not synced to instances.

## Version Naming Conventions

You can use any string as a version identifier:

- Semantic versioning: `v1.0.0`, `v1.2.3`, `v2.0.0-beta`
- Date-based: `2024-01-15`, `20240115-1430`
- Git-based: `main-abc123`, `feature-xyz`
- Environment: `dev`, `staging`, `prod`
- Custom: `tournament-finals`, `hotfix-crash`

## Default Version

Create a `default` version for fallback:

```bash
aws s3 sync ./v1.0.0 s3://my-bucket/builds/default/
```

This version is used when no build version is specified in the game session request.

## Common Issues

### "Permission denied" when starting server
- Ensure `gameserver` is executable: `chmod +x gameserver`
- Check file ownership matches container user (UID 1000)

### Server crashes immediately
- Test the binary locally first
- Check CloudWatch logs for error messages
- Verify all dependencies are included
