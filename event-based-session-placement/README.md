# Event-Based Session Placement for Amazon GameLift Servers

Guidance on best practices for implementing event-driven game session placement using Amazon GameLift Queues.

## Overview

This guidance demonstrates how to build a scalable, event-based session placement system using [Amazon GameLift Servers Queues](https://docs.aws.amazon.com/gameliftservers/latest/developerguide/queues-intro.html). It comes with an AWS Cloud Development Kit (CDK) template for deploying an Amazon GameLift Servers Queue, an Amazon Simple Notification Service (SNS) Topic for queue events, and an AWS Lambda function for processing the events.

The guidance also goes through best practices of utilizing queues, as well as key features to consider when implementing game session placement.

## Prerequisites

The deployment can be done entirely using [AWS CloudShell](https://aws.amazon.com/cloudshell/). To test it locally, you will need the following tools:

- [AWS CLI installed and configured](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-getting-started.html) with valid credentials and permissions at minimum to Amazon GameLift Servers, AWS Lambda, Amazon SNS, AWS CloudFormation, Amazon CloudWatch, and Amazon DynamoDB
- [Node.js 18+ and npm installed](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm)
- [AWS CDK installed](https://docs.aws.amazon.com/cdk/v2/guide/getting-started.html) globally

## Setup

1. Clone the repository:
```bash
git clone https://github.com/amazon-gamelift/amazon-gamelift-toolkit.git
cd event-based-session-placement
```

2. Install dependencies:
```bash
npm install
```

3. Bootstrap CDK (if not done previously):
```bash
cdk bootstrap
```

## Deployment

### Deploy the Stack

```bash
cdk deploy
```

## Integrating with your Amazon GameLift Servers Fleet

To actually make use of the deployed infrastructure, you need an Amazon GameLift Servers Fleet running your game server binary. A quick way to get an existing game server running on Amazon GameLift Servers is the [Containers Starter Kit](https://github.com/amazon-gamelift/amazon-gamelift-toolkit/tree/main/containers-starter-kit) which you can find as part of this same repository.

Once you have your fleet up and running, you can add it to the Queue as a *Destination* by editing the Queue in the AWS Management Console. You can optionally set the fleet destinations already as part of the deployment by modifying the `gameSessionQueue` under `lib/gamelift-queue-stack.ts` and redeploying the CDK stack. The lines for editing are commented out

```typescript
      // You can add the fleet destinations here, or add them later to the Queue
      //destinations: [{
      //  destinationArn: "fleet-arn-here",
      //}],
```

Only after registering a fleet to the Queue will you get successful placements. A successful placement requires the fleet to have a free game server process available and the game session activating the session when it receives the callback to host a new game session.

## Understanding Queues and the Game Session Placement Flow

Amazon GameLift Servers Queues is a free built-in feature of the service. It places game session requests to one or more fleets registered to the Queue. The main purposes for using queues include:

* Placing game sessions based on player latency on optimal location
* Balancing placements between Spot and On-Demand fleets
* Failover to secondary locations in case first choice is not available
* Event-based placement with notifications on placement success and failures
* Retries within the timeout configuration to find a placement

The game session placement flow starts with your game backend requesting a game session through the Queue. This is done with the `StartGameSessionPlacement` API that is available in any AWS SDK of your choice. In the samples below we use the AWS CLI to call this API.

There are three options for calling this API:

* Requesting a session with [PlayerLatencies](https://docs.aws.amazon.com/gameliftservers/latest/apireference/API_StartGameSessionPlacement.html#gameliftservers-StartGameSessionPlacement-request-PlayerLatencies) defined, which will make the Queue place the session based on the `playerLatencyPolicies` defined in the `gameSessionQueue` under `lib/gamelift-queue-stack.ts`
* Requesting a placement that overrides the latency policies and places session based on a `PriorityConfigurationOverride` that defines a fixed list of locations in priority order
* Requesting a session without providing either one of the options above. This only works with Queues that don't have `playerLatencyPolicies` and have defined a location priority order. It is a rarely used option because you're not taking into account the location of the players, and works best for single location fleets.

## Architecture

![Architecture Diagram](Architecture.png)

### Key Components

#### 1. Amazon GameLift Game Session Queue

The Queue receives placement requests from your game backend and intelligently places game sessions across one or more registered fleets. It handles:
- Player latency-based placement using configurable latency policies
- Location priority overrides for custom placement logic
- Automatic retries and failover to secondary locations
- Timeout management for placement requests

#### 2. Amazon SNS Topic

The SNS Topic receives placement event notifications from the GameLift Queue. Events are published for:
- `PlacementFulfilled` - Successful session placement with connection details
- `PlacementFailed` - Failed placement attempts
- `PlacementTimedOut` - Placements that exceeded the timeout period
- `PlacementCancelled` - Manually cancelled placements

#### 3. AWS Lambda Function

The Lambda function subscribes to the SNS Topic and processes all placement events. It:
- Parses incoming placement event data
- Extracts relevant connection information for successful placements (IP, port, DNS)
- Stores placement results in DynamoDB with a 14-day TTL

#### 4. Amazon DynamoDB Table

The DynamoDB table stores placement state for querying by your game backend. It provides:
- Fast lookups by `placementId` for status polling
- Automatic data expiration after 14 days via TTL
- Complete placement history including player session details

## Usage

### Starting Game Session Placement

In the samples below we are using the `StartGameSessionPlacement` API to request a session placement. We are using the AWS CLI for simplicity here, but you can use whatever AWS SDK your game backend is using.

Any request to place a game session requires the `--game-session-queue-name` as well as `--maximum-player-session-count`. The former defines which Queue the placement is sent, and the latter how many players in total can be placed to the game session (the players can be placed as part of the request or later on by creating player sessions for them). The third required field is `--placement-id` that has to always be a unique ID, and using the same ID two times will fail the placement request.

**Using player latencies:**

In this example we're using latency information for two players with player ID's `player1` and `player2`. We are passing the latencies of these two players in just one sample region `us-east-1` but you can pass latencies across all the locations supported by Amazon GameLift Servers. We are also requesting player sessions for both of these players. A good way to measure the player latencies from the client is using the [UDP Ping Beacons](https://docs.aws.amazon.com/gameliftservers/latest/developerguide/reference-udp-ping-beacons.html).

If you have `uuidgen` (most Linux distributions and MacOS do), you can make a test request to session placement with this AWS CLI command:
```bash
aws gamelift start-game-session-placement --game-session-queue-name my-session-placement-queue --maximum-player-session-count 10 --placement-id $(uuidgen) \
--player-latencies "PlayerId=player1,RegionIdentifier=us-east-1,LatencyInMilliseconds=20" "PlayerId=player2,RegionIdentifier=us-east-1,LatencyInMilliseconds=30" \
--desired-player-sessions "PlayerId=player1" "PlayerId=player2"
```

If you don't have `uuidgen` on your system, use this command and make sure to replace the `--placement-id` with a unique identifier each request:
```bash
aws gamelift start-game-session-placement --game-session-queue-name my-session-placement-queue --maximum-player-session-count 10 --placement-id 1234 \
--player-latencies "PlayerId=player1,RegionIdentifier=us-east-1,LatencyInMilliseconds=20" "PlayerId=player2,RegionIdentifier=us-east-1,LatencyInMilliseconds=30" \
--desired-player-sessions "PlayerId=player1" "PlayerId=player2"
```

**Overriding placement locations:**

Sometimes you don't want to use latencies, but rather just give a list of desired locations in priority order to host the session. It could be that you already have done the selection of best hosting locations in your backend, or your game design requires selecting a specific location for hosting.

The `StartGameSessionPlacement` API offers an override that works on Queues where `Location` is set as the first priority for placement. You can enable this in `bin/app.ts` by setting `prioritizeLocation: true` for the stack. See `lib/gamelift-queue-stack.ts` to see how the priority is set with this flag.

When you create a game session, you then define the `--priority-configuration-override` with the desired location order and pass the `PlacementFallbackStrategy` to either only accept locations in your list (`NONE`) or use the other locations in the queue after trying your defined locations (`DEFAULT_AFTER_SINGLE_PASS`).

```bash
aws gamelift start-game-session-placement --game-session-queue-name my-session-placement-queue --maximum-player-session-count 10 --placement-id $(uuidgen) \
--priority-configuration-override LocationOrder=["us-east-1","us-east-2","us-west-2"],PlacementFallbackStrategy=NONE \
--desired-player-sessions "PlayerId=player1" "PlayerId=player2"
```

As above, you can also use a fixed `--placement-id` as long as it's unique.

## Utilizing Queue Events to Inform Players

The system automatically processes [game session placement events](https://docs.aws.amazon.com/gameliftservers/latest/developerguide/queue-events.html) and stores the placement results in Amazon DynamoDB table `my-queue-session-placement-state`. You can query the DynamoDB table in your backend with any AWS SDK to provide players information on the results of the placement requests. There is an AWS Lambda function processing the requests and in case you're using a WebSocket connection between your client and the server, it's also possible to trigger a response directly to the clients by notifying your WebSocket handling system from the AWS Lambda function directly. With the sample implementation we are expecting the client to poll your backend for the data.

See [DynamoDB Data Model](#dynamodb-data-model) below to understand the structure of the data stored on session placement events. For each player in the match, you would typically pass on the `ipAddress`, `port`, as well as the correct `playerSessionId` extracted from the JSON data in `placedPlayerSessions`. PlayerSessionId can be used to validate the player session on the server side.

## DynamoDB Data Model

The placement events are stored in a DynamoDB table with the following schema:

**Table Name:** `my-queue-session-placement-state`

**Primary Key:**
- Partition Key: `placementId` (String) - Unique identifier for the placement request
- Sort Key: `timestamp` (String) - ISO 8601 timestamp when the event was processed

**Attributes stored for all events:**
- `type` (String) - Event type: `PlacementFulfilled`, `PlacementCancelled`, `PlacementTimedOut`, or `PlacementFailed`
- `queueHomeRegion` (String) - AWS region where the queue is located
- `startTime` (String) - ISO 8601 timestamp when placement started
- `endTime` (String) - ISO 8601 timestamp when placement completed
- `rawEventData` (String) - Complete JSON of the original event for debugging
- `ttl` (Number) - Time-to-live attribute set to 14 days from event creation. Items will be deleted from the table after this time

**Additional attributes for PlacementFulfilled events:**
- `port` (String) - Port number for connecting to the game session
- `ipAddress` (String) - IP address of the game server
- `dnsName` (String) - DNS name of the game server
- `gameSessionRegion` (String) - AWS region where the game session was placed
- `gameSessionArn` (String) - ARN of the created game session
- `placedPlayerSessions` (String) - JSON array of player session details for players included in the placement

Example contents of the `placedPlayerSessions` attribute:

```json
[{"playerId":"player1","playerSessionId":"psess-63f415d2-71d9-6ecd-c479-d066ccab3ced"},
{"playerId":"player2","playerSessionId":"psess-63f415d2-71d9-6ecd-c479-d066ccab40af"}]
```

## Configuration

The deployed AWS resources can be configured in `lib/gamelift-queue-stack.ts`. Main things to look into include:
- Queue timeout
- Queue latency policies
- Placement handler AWS Lambda function resources
- Fleet destinations (fleets are deployed separately from this sample)

To enable using the location override, set the `prioritizeLocation` flag in `bin/app.ts` to `true`.

## Monitoring

Queues will emit [metrics](https://docs.aws.amazon.com/gameliftservers/latest/developerguide/monitoring-cloudwatch.html) to Amazon CloudWatch. It's possible to create CloudWatch Alarms from these metrics to notify any internal systems you have for on-call and 24/7 monitoring. 

Some of the key metrics to monitor include `AverageWaitTime`, `QueueDepth`, `PlacementsFailed`, and `PlacementsTimedOut`. These are good candidates for creating alarms on for situations where placements start to fail and time out more than usual, or if the wait time or depth of the queue goes over a threshold.

The [Development phase steps for successful launches on Amazon GameLift Servers](https://aws.amazon.com/blogs/gametech/development-phase-steps-for-successful-launches-on-amazon-gamelift-servers/) blog post covers metrics, logs, and alarms in more detail.

## Cleanup

To remove all deployed resources and avoid ongoing charges:

```bash
cdk destroy
```

Note: This will delete the DynamoDB table and all placement history. The GameLift fleets registered to the queue are managed separately and will not be deleted.

## License

This project is licensed under the MIT License.
