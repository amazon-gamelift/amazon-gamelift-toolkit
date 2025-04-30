# Building the Amazon GameLift Server SDK for Unreal Engine 5 on Amazon Linux

This quick guide shows how to build the binaries for the Amazon GameLift Server SDK for Unreal Engine 5 builds on Amazon Linux 2023. The build is done in the AWS Cloud Shell without the need to install any additional tools on your local system. The output binaries can be used with the Amazon GameLift Servers Plugin for Unreal Engine.

**NOTE:** The fastest way to run the build is to use AWS CloudShell with the instructions below without the need to install any tools locally. If you do however want to run it locally, the only things you need a [git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git) and [docker](https://docs.docker.com/get-docker/) installed and you can run the build on MacOS terminal or Windows [WSL](https://learn.microsoft.com/en-us/windows/wsl/install). 

# Contents

This sample consists of a deployment script and a configuration file:

* **`Dockerfile`**: The dockerfile that builds the GameLift Server SDK and OpenSSL on Amazon Linux
* **`buildbinaries.sh`**: A simple shell script that executes the docker file and creates a zip-file with the binaries

# Notes on customizing to your needs

It's important to use the same OpenSSL version as your Unreal Engine 5 version uses. You can find this information by going to `Engine/Source/Thirdparty/OpenSSL` in your Unreal Engine source, where the include folder will have a prefix such as `1.1.1n`. The `Dockerfile` downloads this version as it's compatible with UE 5.1, 5.2 and 5.3. You might need to change it for older or newer UE5 versions.  

# Building the SDK

**Open** the AWS Management Console, and make sure your region is N.Virginia for this example. Then launch AWS CloudShell:

![AWS CloudShell](../development-instance-with-amazon-gamelift-anywhere-and-gamelift-agent/CloudShell.png)

**Clone** this repository and open the correct folder by running the following command in CloudShell:

```bash
git clone https://github.com/aws/amazon-gamelift-toolkit.git
cd amazon-gamelift-toolkit/building-gamelift-server-sdk-for-unreal-engine-and-amazon-linux/
```

**Run** the following command in CloudShell to build the SDK:

```bash
./buildbinaries.sh
```

**Select** `Actions` and `Download` in CloudShell and type `/home/cloudshell-user/amazon-gamelift-toolkit/building-gamelift-server-sdk-for-unreal-engine-and-amazon-linux/AL2023GameliftUE5sdk.zip` to download the binaries to your local system.
# Quick start with Amazon GameLift Servers Unreal Plugin

The Amazon GameLift Servers Unreal plugin supports Linux-based deployments as long as you have the correct binaries available. If you feel like you already know what you're doing and only want to get the binaries to the right place, these steps help you do exactly that:

1. Follow the steps in [Building the SDK](#building-the-sdk) to build the SDK and download the binaries
2. Copy the `libaws-cpp-sdk-gamelift-server.so` to `amazon-gamelift-plugin-unreal/GameLiftPlugin/Source/GameliftServer/ThirdParty/GameLiftServerSDK/Linux/x86_64-unknown-linux-gnu/` inside the Amazon GameLift Servers Unreal plugin in your project
3. Once you've packaged the project for Linux, copy the files `libcrypto.so-.1.1` and `libssl.so.1.1` to your package folder under `<YOURGAME>/Binaries/Linux` before uploading the build to Amazon GameLift Servers

See the next section for a detailed step by step guide.

# Step by step instructions with Amazon GameLift Servers Unreal Plugin

These are the more detailed steps on setting up your Unreal Engine project with the Amazon GameLift Servers Plugin and Linux binaries to deploy a Linux fleet on Amazon GameLift Servers. Most of the steps are the same as in the [guide for integrating games with the plugin for Unreal](https://docs.aws.amazon.com/gamelift/latest/developerguide/unreal-plugin.html), but we have some important modifications for the Linux game server setup>

1. Follow the steps to build the SDK and download in Cloud Shell
2. Build UE5 from source (DevelopmentEditor configuration) to start the editor
3. Install the [cross-compile toolkit for your UE version](https://dev.epicgames.com/documentation/en-us/unreal-engine/linux-development-requirements-for-unreal-engine?application_version=5.4)
4. Create a new C++ based game project (3rd person), or use your existing game project with C++ enabled
5. Download the [GameLift Unreal Plugin](https://github.com/aws/amazon-gamelift-plugin-unreal/releases/tag/v1.1.1)
  * Unzip the plugin, and then unzip the `amazon-gamelift-plugin-unreal-1.1.1-sdk-5.1.1.zip`
  * We don’t need the SDK zip as we already built that
6. Copy the `libaws-cpp-sdk-gamelift-server.so` which we built before to `amazon-gamelift-plugin-unreal/GameLiftPlugin/Source/GameliftServer/ThirdParty/GameLiftServerSDK/Linux/x86_64-unknown-linux-gnu/`
7. Copy the `GameLiftPlugin` folder from the `gamelift-plugin-unreal` folder to the Plugins folder in the game project directory. (we’re following the instructions [here](https://docs.aws.amazon.com/gamelift/latest/developerguide/unreal-plugin-install.html)). You’ll need to create the plugins folder if you don’t have it yet
8. Add these to your uplugin project file under Plugins:
```json
    {
        "Name": "GameLiftPlugin",
        "Enabled": true
    },
    {
         "Name": "WebBrowserWidget",
        "Enabled": true
    }
```
9. Build the game project in Visual Studio
10. Run your game project with “DevelopmentEditor” configuration to spin up the editor. For existing game project you might need to change the project to use the Unreal source code version.
11. Follow Step 1 and 2 only from the [Unreal Plugin Anywhere setup](https://docs.aws.amazon.com/gamelift/latest/developerguide/unreal-plugin-anywhere.html) to set up your profile, set up your game mode code, integrate your client game map, and build your game (the AWS profile setup before this is optional for our needs). Note on the following:
  * For setting the maps, make sure you select the settings icon and “Show plugin content” to find the sample startup map
  * You will need to restart the editor to get the build targets showing correctly
12. We need to fix the m_processId definition in your game mode CPP file (somewhere around line 82) to work correctly on Linux. Replace it with this:
```cpp
        else
        {
            // If no ProcessId is passed as a command line argument, generate a randomized unique string.
            FString TimeString = FString::FromInt(std::time(nullptr));
            FString ProcessId = "ProcessId_" + TimeString;
            ServerParametersForAnywhere.m_processId = TCHAR_TO_UTF8(*ProcessId);
        }
```
13. Package the project for Linux. In the Editor select “Platforms”, then select “Linux” and select your Server build target. Then select “Package project”
14. Once it’s packaged, copy the files `libcrypto.so-.1.1` and `libssl.so.1.1` to your package folder under `<YOURGAME>/Binaries/Linux`
15. Create an install.sh file in the root of the build. Replace `<YOURGAME>`  and `<YOURGAMEBINARY>` with the correct folder and binary name. Make sure you have Unix line endings in the script by following a guide like this one if you’re creating on Windows.
```bash
#!/bin/bash

sudo chmod 777 /local/game/<YOURGAME>/Binaries/Linux/<YOURGAMEBINARY>

```     
16. Set up the Amazon GameLift Servers Unreal Plugin profile
17. Follow the guide for [deploying a managed Amazon GameLift Servers fleet with the plugin](https://docs.aws.amazon.com/gamelift/latest/developerguide/unreal-plugin-ec2.html) to deploy a test fleet. (You could also manually upload the build using the AWS CLI and use any method you want for creating a fleet). Make sure to do the following changes to deployment for Amazon Linux 2023
  * Set the `Server Build OS`to `Amazon Linux 2023 (AL2023`) in the UI
  * Manually input the `Server build executable` (this has to be an absolute path)
18. Build and run a Windows client using the plugin


