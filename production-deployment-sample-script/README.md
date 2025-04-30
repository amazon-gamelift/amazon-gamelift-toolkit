Overview
--------
The purpose of this script is to help you deploy a game server update for Amazon GameLift Servers hosting and then transition
player traffic to game sessions running on the new game server.  The mechanics of the script rely on the use of an Alias to designate a Fleet resource.  With this script, you upload the game server update as a new Build resource and deploy it to a new Fleet resource. The script then updates the Alias assignment from the current Fleet to the new Fleet, once the new Fleet is ready to accept player traffic.  At that point, players are directed to join game sessions that use the new version of the game server.

Prerequisites
-------------
1. Install AWS SDK for Python: https://aws.amazon.com/sdk-for-python/
1. Install python packages for unit testing:
   1. `pip install pytest`
   2. `pip install mock`
   3. `pip install pytest-mock`

Limitations
-----------
This script offers a sample for uploading game server updates in a production environment. As your game hosting architecture matures to encompass multiple Fleets and AWS Regions, you'll need to customize the script to fit your deployment needs.

Keep in mind the following limitations, including best practices that the script doesn't currently handle:
* The script requires that your game uses [Aliases](https://docs.aws.amazon.com/gamelift/latest/developerguide/aliases-intro.html) to designate the destination of your game sessions, either in your GameSessionQueue or in your game's call to CreateGameSession.
* The script only supports game server uploads from your own S3 Bucket.
* You must input the specific resources you want to update.  As a customization, you could use tags to list and find specific resources to replace in a more automated approach.
* When replacing an existing Fleet with a new Fleet, be sure that the new Fleet's capacity matches the old Fleet to handle player traffic when redirected.  This can be done by modifying the script to call [UpdateFleetCapacity](https://docs.aws.amazon.com/gamelift/latest/apireference/API_UpdateFleetCapacity.html), applying a scaling policy with [PutScalingPolicy](https://docs.aws.amazon.com/gamelift/latest/apireference/API_PutScalingPolicy.html), or both.  This should be done after the Fleet is ACTIVE and before updating the Alias.
* The script does not have features like error detection, resuming execution, or rollback options.

Required Arguments
------------------
| Name | Short Syntax | Description |
| ---- | ------------ | ----------- |
| --region | -r | The AWS Region the Fleet and Alias resources are located in. |
| --fleet-id | -f | The FleetId of the existing Fleet being replaced. |
| --alias-id | -a | The AliasId of the existing Alias being updated to direct traffic to the new Fleet. |
| --build-json | -bj | A json file detailing the Build resource to be created. |
| --fleet-json | -fj | A json file detailing the Fleet resource to be created. |

Example Usage
-------------
1. Find the existing Fleet to replace: `aws gamelift describe-fleet-attributes --region <region>` to find the FleetId.
   1. Learn more about DescribeFleetAttributes [here](https://docs.aws.amazon.com/gamelift/latest/apireference/API_DescribeFleetAttributes.html)
1. Find the existing Alias to update and direct traffic to the new Fleet: `aws gamelift list-aliases --region <region>` to find the AliasId.
   1. Learn more about ListAliases [here](https://docs.aws.amazon.com/gamelift/latest/apireference/API_ListAliases.html)
1. Create a json skeleton for Build resource: `aws gamelift create-build --generate-cli-skeleton > new_build.json`
1. Create a json skeleton for Fleet resource: `aws gamelift create-fleet --generate-cli-skeleton > new_fleet.json`
1. Fill in Build and Fleet attributes in those new json files.  Ideally this only needs to be done once and future updates will have minimal attributes to change on each revision.
   1. Learn more about CreateBuild [here](https://docs.aws.amazon.com/gamelift/latest/apireference/API_CreateBuild.html)
   1. Learn more about CreateFleet [here](https://docs.aws.amazon.com/gamelift/latest/apireference/API_CreateFleet.html)
1. Execute the script:
```
python3 ./deployment.py -r us-west-2 \
--fleet-id fleet-12345678-1234-1234-1234-12345678 \
--alias-id alias-12345678-1234-1234-1234-12345678 \
--build_json new_build.json \
--fleet_json new_fleet.json
```

How It Works
------------
The script takes the following actions:
1. Validate that the input Fleet and Alias exist with `describe-fleet-attributes` / `describe-alias`.
1. Call `create-build` based on the input Build json file to create the new Build.
1. Loop calling `describe-build` until the new Build is in state READY.
1. Call `create-fleet` based on the input Fleet json file to create the new Fleet.
1. Loop calling `describe-fleet-attributes` until the new Fleet state is ACTIVE.
1. Loop calling `describe-fleet-location-attributes` until all new Fleet Locations are ACTIVE.
1. Call `update-alias` to update the Alias with the new Fleet and send player traffic to hardware running the new game server.
1. Loop calling `describe-game-sessions` on the old Fleet until there are none remaining.
1. Call `delete-fleet` on the old Fleet to cleanup the old game server version when it is no longer in use.

Running unit tests
------------------
Unit tests are written using pytest, simply run from the parent directory:
```
pytest
```
