import * as cdk from 'aws-cdk-lib';
import * as gamelift from 'aws-cdk-lib/aws-gamelift';
import * as sns from 'aws-cdk-lib/aws-sns';
import * as snsSubscriptions from 'aws-cdk-lib/aws-sns-subscriptions';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import { Construct } from 'constructs';

export interface GameLiftQueueStackProps extends cdk.StackProps {
  prioritizeLocation?: boolean;
}

export class GameLiftQueueStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props?: GameLiftQueueStackProps) {
    super(scope, id, props);

    // SNS Topic for placement events
    const placementTopic = new sns.Topic(this, 'PlacementTopic', {
      topicName: 'my-queue-placement-events',
    });

    // DynamoDB table for session state
    const sessionStateTable = new dynamodb.Table(this, 'SessionStateTable', {
      tableName: 'my-queue-session-placement-state',
      partitionKey: { name: 'placementId', type: dynamodb.AttributeType.STRING },
      sortKey: { name: 'timestamp', type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
      timeToLiveAttribute: 'ttl',
    });

    // Game Session Queue
    const gameSessionQueue = new gamelift.CfnGameSessionQueue(this, 'GameSessionQueue', {
      name: 'my-session-placement-queue', // The name is used when requesting session placement
      timeoutInSeconds: 60, // 1 minute timeout
      notificationTarget: placementTopic.topicArn, // send notification to the SNS topic
      // Latency policies total of 15 seconds starting from 150ms to 10000ms (any latency)
      playerLatencyPolicies: [{
        // Policy 1: Max 150ms latency for the first 5 seconds
        policyDurationSeconds: 5,
        maximumIndividualPlayerLatencyMilliseconds: 150
      },{
        // Policy 2: Max 250ms latency for 5 seconds
        policyDurationSeconds: 5,
        maximumIndividualPlayerLatencyMilliseconds: 250
      },
      {
        // Policy 3: Accept any latency after 5 more seconds (still requires backend to provide the latency values)
        policyDurationSeconds: 5,
        maximumIndividualPlayerLatencyMilliseconds: 10000
      }],
      // If we have enabled location priority, use it, otherwise we just use the default that prioritizes latency and cost
      ...(props?.prioritizeLocation && {
        priorityConfiguration: {
          priorityOrder: ['LOCATION', 'LATENCY', 'COST', 'DESTINATION'],
          locationOrder: [this.region] // Note, we have to have a location order so we just reference the queue home region here, you could define a default order for all locations here but we are planning to override that in every session placement request
        },
      }),
      // You can add the fleet destinations here, or add them later to the Queue
      //destinations: [{
      //  destinationArn: "arn:aws:gamelift:REGION:ACCOUNT:fleet/fleet-xxxxx",
      //}],
    });

    // Lambda for session placement
    const placementHandler = new lambda.Function(this, 'PlacementHandler', {
      runtime: lambda.Runtime.NODEJS_18_X,
      handler: 'placement-handler.handler',
      code: lambda.Code.fromAsset('lambda'),
      environment: {
        TABLE_NAME: sessionStateTable.tableName,
      },
      memorySize: 1024,
      timeout: cdk.Duration.seconds(30)
    });

    // Grant Lambda permissions to write to DynamoDB
    sessionStateTable.grantWriteData(placementHandler);

    // Subscribe Lambda to SNS topic
    placementTopic.addSubscription(new snsSubscriptions.LambdaSubscription(placementHandler));

    // Grant GameLift permission to publish to SNS topic
    placementTopic.addToResourcePolicy(new iam.PolicyStatement({
      effect: iam.Effect.ALLOW,
      principals: [new iam.ServicePrincipal('gamelift.amazonaws.com')],
      actions: ['sns:Publish'],
      resources: [placementTopic.topicArn],
    }));

    // Output the game session queue ARN
    new cdk.CfnOutput(this, 'GameSessionQueueArn', {
      value: gameSessionQueue.attrArn,
      description: 'ARN of the GameLift game session queue',
    });
  }
}
