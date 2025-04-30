# Fast Build Update Tool

# Overview

`Fast Build Update Tool` is a command-line application that can be used to quickly update a game server build in your Amazon GameLift Servers fleet by bypassing the typical build and release process supported by Amazon GameLift Servers.

Typically updating a build in Amazon GameLift Servers requires uploading the build, creating a new fleet, waiting for the fleet to transition to ACTIVE, and then redirecting any traffic to the new fleet. This process is a great way to manage updates in most cases (production, QA, etc), but can be quite time-consuming during development when you want to quickly iterate on code and changes.

With the Fast Build Update Tool, you can copy and overwrite a game server build onto one or more instances in an existing fleet, and then restart any running game server processes with your changes.

Special thanks to [Rushdown Studios](https://www.rushdownstudio.com/) for developing this tool in consultation with the Amazon GameLift Servers team.

## Glossary

Many terms around Amazon GameLift Servers and game servers can be used interchangeably or have more than one meaning. We will use the following language for consistency:

* **Build**: The file or files that are used to run your game server.
* **Executable**: The main executable that is used to start your game server.
* **ServerProcess**: A running game server executable.
* **Instance**: An EC2 Instance running in a GameLift fleet. An instance may have one or more game server processes running on it.

## ⚠️Limitations and Recommendations⚠️

1. **❗IMPORTANT** This tool should only be used for development, and internal environments **only**! It is highly recommended that any player-facing builds continue to use the normal build release process supported by Amazon GameLift Servers!
1. This tool updates builds on instances that are currently deployed in a fleet. Any new instances that are added to the fleet (such as scaling out or replacing an instance) will be deployed with the original uploaded build. We very strongly recommend using this tool to quickly test updates as a complement to the normal build release process, **even in development environments**. We recommend using this tool with:
    * Static fleets that do not auto-scale new instances. New instances will run the original build uploaded to Amazon GameLift Servers, not your updated version from this tool.
    * On-Demand Instances. If you use Spot Instances, you will lose changes that you have uploaded with this tool if the instance is interrupted and replaced.
1. This tool bypasses some of the protections provided by Amazon GameLift Servers when you upload a build and create a new fleet. If this tool is used improperly, or is run with a broken server build, instances in your fleet could enter a broken state. Since this tool is meant for development only, scale the fleet down to 0 instances and back up to return the fleet to a healthy state with your original uploaded build.
1. Only one execution of this tool should be run against a single fleet at a time.
1. If possible, try to keep the size of your server builds small. This tool works by copying a game server build to each instance in the fleet individually. If you have very large server builds, this can be a time-consuming operation.
    * This tool supports partial build updates. If you confidently know which files have changed between your local build and the build running on the instance, you can actually call this tool with a `zip` file containing: any files that have changed, and the executable files defined in the runtime configuration of the fleet. If you decide to do a partial update, it is **CRUCIAL** that the location of these zipped files **exactly** matches the location of these files in the build that was originally uploaded!
1. In order for this tool to work, it automatically opens a port on your fleet for a range of IP addresses specified by you. It does not remove this access after it has finished running. If you would like to close this port, you will currently have to do so by updating the fleet's EC2 port settings either through the Amazon GameLift Servers console or the AWS CLI (`aws gamelift update-fleet-port-settings`).


## How it Works

The basic flow this tool follows is:
* Discover each instance in a fleet.
* Open an SSH port on the fleet to a range of IP addresses specified by you.
* For each instance in the fleet:
    * Gain remote access to the instance through SSM.
    * Enable SSH on the instance.
    * Copy your updated build and any related files to the instance over SSH.
    * Replace any existing build files on the instance with your updated build files.
    * Restart any game server processes on the server with the new build.

## Current Compatibility

**Key**:
🔴 Not functional
🟢 Fully functional


| Operating System    | Amazon GameLift Server SDK < 5 | Amazon GameLift Server SDK v5 |
|---------------------|--------------------------------|-------------------------------|
| Amazon Linux 2      | 🔴                             | 🔴                            |
| Amazon Linux 2023   | 🔴                             | 🟢                            |
| Windows Server 2012 | 🔴                             | 🔴                            |
| Windows Server 2016 | 🔴                             | 🟢                            |

| Operating Platform | |
|--------------------| ------- |
| Linux              | 🟢 |
| Windows            | 🟢 |


# How to Use the Tool

## Pre-Requisites

**IMPORTANT You will need the following dependencies installed and set up correctly on your local machine in order to run this tool. If you are missing any of these things this tool will not function!**

1. **Game Server Build**
    * You will need access to a server build for your game.
    * This tool requires that the build is compressed to a `.zip` file (there are more detailed instructions on how to set this up later in the document).
1. **Fleet resource**
    * To take advantage of this tool you must have a pre-existing Amazon GameLift Servers fleet that runs on managed EC2 instances.
1. **Go**
    * This project is written in Go. You will need Go 1.21.11 or newer compile the source. [Instructions to download and install Go can be found here.](https://go.dev/doc/install)
1. **AWS CLI**
    * You will need to have the [AWS CLI](https://aws.amazon.com/cli/) installed on your local machine.
    * Make sure you have the [default region configured](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html) as the tool utilizes that to define the fleet location.
1. **AWS CLI SSM Plugin**
    * Amazon GameLift Servers uses [SSM](https://docs.aws.amazon.com/systems-manager/latest/userguide/ssm-agent.html) to manage remote instance connections. **[In order to use SSM you will need to install the SSM CLI plugin from Amazon. You can find instructions to do this here.](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)**
1. **Valid IAM Credentials**
    * You must have valid IAM credentials in order to run this tool.
    * This tool looks for AWS credentials in the default locations supported by the AWS CLI (environment variables, `~/.aws/credentials`, etc...). [The different configuration options are outlined here.](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html)
    * You can find information on how to change permissions for a user [here](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_users_change-permissions.html).
    * You must be able to take the following IAM actions against your fleet. 
        * `gamelift:DescribeFleetAttributes`
        * `gamelift:UpdateFleetPortSettings`
        * `gamelift:DescribeInstances`
        * `gamelift:DescribeFleetLocationAttributes`
        * `gamelift:GetComputeAccess`
        * `gamelift:DescribeRuntimeConfiguration`
1. **SSH Client**
    * You will need an SSH client with SCP installed on your local machine. Most Linux distros have SSH pre-installed. Windows users can either install Git for Windows which comes bundled with OpenSSH, or install OpenSSH separately.
1. **Windows Client Only: ConPTY**
    * A version of Windows that supports ConPTY ([Windows 10 October 2018 Update (version 1809) or newer](https://learn.microsoft.com/en-us/windows/console/createpseudoconsole))

## SSH Key Setup

Proper setup of your SSH key is crucial for this tool to function correctly.

Windows Users:
- Store your private key file in a secure location, such as your Documents folder.
- Ensure the key file has the correct permissions. You can check this with:
  icacls "C:\Users\YourUsername\Documents\MyPrivateKey.pem"

Linux Users:
- Set the correct permissions on your key file:
  chmod 400 MyPrivateKey.pem
- Verify the permissions with:
  ls -l MyPrivateKey.pem
    
## Downloading & Compiling the `Fast Build Update Tool`

**Make sure you have all the pre-requisites outlined in the previous section before continuing.**

The tool must be downloaded from GitHub and compiled before it can be run.

You can download the source code of this repository from GitHub by following the instructions [HERE](https://docs.github.com/en/repositories/working-with-files/using-files/downloading-source-code-archives).

To compile the source, run the following commands in the base directory of the repository in your terminal of choice. 

Linux:
```sh
# Download any dependencies required by this tool
go mod download
# Build the Linux executable as fastbuild
go build -o fastbuild cmd/main.go
# Verify everything compiled correctly
./fastbuild --help
```

Windows:
```ps1
# Download any dependencies required by this tool
go mod download
# Build the Windows executable as fastbuild.exe
go build -o fastbuild.exe cmd/main.go
# Verify everything compiled correctly
.\fastbuild.exe --help
```

After following these steps you should have a working executable of the tool (the file will either be `fastbuild` or `fastbuild.exe` depending on your platform). You can check that everything was compiled correctly by using the help option.

## Preparing your Game

### Finding your Fleet ID

You will need to know the fleet ID and AWS Region of the fleet you are trying to update in order to run this tool. The simplest way to find your fleet ID is by looking in the Amazon GameLift Servers console (the fleet ID starts with the `fleet-` prefix).

If you do not have access to the AWS web console, you can find your fleet using the AWS CLI. First list all fleets available in your region:

```sh
aws gamelift list-fleets
# Example output:
# {
#     "FleetIds": [
#         "fleet-a1b2c3d4-5678-90ab-cdef-EXAMPLE11111",
#         "fleet-a1b2c3d4-5678-90ab-cdef-EXAMPLE33333",
#         "fleet-a1b2c3d4-5678-90ab-cdef-EXAMPLE44444"
#     ]
# }
```

Get more info about a specific fleet listed above:

```sh
aws gamelift describe-fleet-attributes --fleet-ids fleet-a1b2c3d4-5678-90ab-cdef-EXAMPLE11111
# Example output:
# {
#     "FleetAttributes": [
#         {
#             "FleetId": "fleet-a1b2c3d4-5678-90ab-cdef-EXAMPLE11111",
#             "Name": "my fleet",
#             "BuildId": "build-11111111-1111-1111-1111-111111111111"
#             # Abbreviated here for the sake of space
#         }
#     ]
# }
```

If you do not see the target fleet in the above output, you may need to change your AWS region (using the `AWS_REGION` environment variable or the `--region` command line argument).

### Generating a Zip Archive of the Server Build

This tool requires a `.zip` file containing your game server build as input. This is the build that will be copied to any instances in your fleet.

The path to the executable in the zip file must match the launch path in the runtime configuration that the fleet was initially configured with.

As an example, if you originally uploaded a build to Amazon GameLift Servers with the following directory structure:

```sh
├── bin
|   ├── mygame.exe
|   ├── dependency.dll
```

You would need to produce a `zip` file with the same file and directory structure. The `bin` folder must be at the top level of the `zip` file, and must contain `mygame.exe`. Each folder must also contain any relevant files for the build. It is important to verify that you have not introduced additional folder nesting inside the `zip` file!

**Linux** users can use the `zip` command in their shell of choice:
```sh
cd ./build-folder
zip -r ../mygame.zip .
```

**Windows** users can create a `zip` file through File Explorer, or by using the [`Compress-Archive` command in PowerShell](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.archive/compress-archive?view=powershell-7.4):
```ps1
Compress-Archive -Path build-folder/* -DestinationPath "mygame.zip"
```

### Generating a Private SSH Key

This tool requires a valid SSH key in order to connect to the remote instances in your fleet. There are many ways to generate an SSH key. You can generate an SSH key using the AWS CLI:

Linux:
```sh
aws ec2 create-key-pair --key-name fast-build-update-tool --region us-east-1 --query KeyMaterial --output text > MyPrivateKey.pem
```

Windows:
```ps1
aws ec2 create-key-pair --key-name fast-build-update-tool --region us-east-1 --query KeyMaterial --output text | Out-File -Encoding ascii -FilePath .\MyPrivateKey.pem
```

AWS has provided much more detailed instructions on how to generate an SSH Key [here](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/create-key-pairs.html).

### Determining your IP Address

This tool requires a _range_ of **public** IP addresses that you will be running this tool from as input. Any IP address in the range you provide will have access to the SSH port for **all** instances in your fleet. This tool does not automatically revoke access to these IP addresses, after it enables access.

If you do not know your IP address you can look it up using one of the following commands:

Linux:
```sh
my_ip=$(curl https://checkip.amazonaws.com)
echo $my_ip
```

Windows (PowerShell):
```ps1
$my_ip=(Invoke-WebRequest https://checkip.amazonaws.com).Content.TrimEnd()
$my_ip
```

Both Amazon GameLift Servers and this tool use CIDR notation to denote a range of IP addresses. If you would like to lock things down to a single IP address you would simply apply the `/32` suffix to your IP address (`127.0.0.1/32`, or `$my_ip/32`).

If you would like a more complicated setup, you can read more about how CIDR notation works [here.](https://aws.amazon.com/what-is/cidr/#:~:text=CIDR%20notation%20represents%20an%20IP,1.0%2F22.)

## Running the Tool

This is a simple command line tool that can be run from the shell of your choice (Bash, PowerShell, etc..). An example command of running this tool would look like the following:

```sh
./fastbuild --fleet-id=fleet-a1b2c3d4-5678-90ab-cdef-EXAMPLE11111 --ip-range="$my_ip/32" --zip-path=./mygame.zip --private-key=MyPrivateKey.pem
```

### Required Arguments

| Name | Explanation                                                                                                                                                                                                                                                               |
| -------- |---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| --fleet-id | The fleet id of the fleet you would like to update. This tool will currently update every instance within the fleet provided, unless the `instance-ids` argument is provided.                                                                                             |
| --ip-range | The range of local IP addresses from which you will be running this tool.  This is required to open ports for remote access. For access from a single IP you may use the $ip-address/32 format. The SSH port will be opened to **every** IP address in the range provided. |
| --zip-path | The path on your local machine to a server build. The structure inside of the zip file, **MUST** exactly match the structure on your server instances. If the names do not match, this tool will not update your server processes properly!                               |
| --private-key | A private key file that can be used to SSH into a remote instance. If you do not have an existing key you may use the `aws ec2 create-key-pair` command to generate one ([more info here](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/create-key-pairs.html))     |


### Optional Arguments

| Name | Explanation                                                                                                                                                                                                 |
| -------- |-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| --instance-ids | A comma separated list of one or more instance ids you would like to update. Use this argument if you would only like to update specific instances, instead of every instance in a fleet.                   |
| --restart-process | If this flag is passed the tool will only restart the running game server processes, and not actually upload and replace the current build. When this flag is set, the `zip-path` argument must not be set. |
| --ssh-port | **WINDOWS ONLY** Override the port that is used for SSH. This number must be greater than 1025. The default value is 1026. NOTE: Custom SSH ports are not supported for Linux fleets. Linux fleets will always use the default SSH port 22.|
| --verbose | Enable verbose logging instead of the default progress bar display. This can be useful for debugging potential issues.                                                                                      |
              

### Debugging Common Issues

#### `missing required argument`

You are missing one or more the required arguments to start this tool. Please review the [Required Arguments](#required-arguments) section, and try again with the proper arguments provided.

#### `error looking up fleet: fleet not found`

This generally could mean a few things:

1. The fleet-id argument provided was not entered correctly.
1. You are not using the correct AWS credentials, or they have been configured incorrectly. See the `Valid IAM Credentials` section of [Pre-Requisites](#pre-requisites).
1. Your local AWS credentials are configured to point at the wrong region. You can fix this in your `~/.aws/credentials` file, or by setting the `AWS_REGION` environment variable.

#### `error validating zip file: zip file does not contain executable $executableName`

This means that the format of the zip file you are attempting to upload is invalid. The [Generating a Zip Archive of the Server Build](#generating-a-zip-archive-of-the-server-build) section provides detail on how to generate a valid server build.

#### `argument ip-range was invalid: must be a valid IP range`

This means that the `--ip-range` argument was provided with an invalid value. The IP range must follow CIDR notation.

#### `error parsing private key file`

The file provided via the `--private-key` argument is not a valid private SSH key. The [Generating a Private SSH Key](#generating-a-private-ssh-key) section provides detail on how to generate a valid SSH key.

#### SSH Connectivity Issues

If you're experiencing SSH connection problems:

1. Verify SSH port accessibility:
   - For Windows (using PowerShell): 
     Test-NetConnection -ComputerName your-instance-ip -Port (your port)
   - For Linux: 
     telnet your-instance-ip 22

2. Check firewall settings:
   - Ensure your firewall allows outbound connections on port 22 (for Linux) or your specified custom port (for Windows).
   - If using a corporate network, you may need to request SSH port allowance from your IT department.

3. Test direct SSH connection:
   ssh -v -i your-key.pem ec2-user@your-instance-ip

4. For Windows users:
   - Ensure the private key file is stored in a location without special permissions (e.g., My Documents folder).
   - If using Windows Defender Firewall, you may need to add an inbound rule to allow SSH traffic.

5. For Linux users:
   - Remember that custom SSH ports are not supported. Always use the default port 22.

#### Other Issues

There are a number of steps you can take to help debug other issues.

1. Use the `--verbose` flag:
    * This flag provides significantly more detailed output from the tool, and may help you understand what is going wrong.
1. Review log files:
    * By default, this tool writes remote instance logs to the `fast-build-update-tool-logs` folder for the most recent tool run, and `fast-build-update-tool-logs-prev` for the previous run.
    * This folder will contain log output of the remote commands run during the server update process. This may provide insight into issues.
1. Remotely access the instance:
    * Amazon GameLift Servers provides utilities for remotely accessing your instances outlined [here](https://docs.aws.amazon.com/gamelift/latest/developerguide/fleets-remote-access.html).
    * You may be able to diagnose potential issues just by looking around the file system of the remote instance.
        * Windows builds are found in the `C:\game` folder on the remote instance.
        * Linux builds are found in the `/local/game` folder on the remote instance.


# How to Do This Manually / How the Tool Works

We have provided instructions on how to manually update a server executable in place on an Amazon GameLift instance without using this tool.
This can be helpful if your platform of choice is not yet supported by this tool.
These guides can be found in the `docs` folder [HERE](docs/00_ManualUpload.md).
