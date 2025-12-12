#!/usr/bin/env node
import 'source-map-support/register';
import * as cdk from 'aws-cdk-lib';
import { GameLiftQueueStack } from '../lib/gamelift-queue-stack';

const app = new cdk.App();

new GameLiftQueueStack(app, 'GameLiftQueueStack', {
  env: {
    account: process.env.CDK_DEFAULT_ACCOUNT,
    region: process.env.CDK_DEFAULT_REGION,
  },
  prioritizeLocation: false, // Set to true to prioritize location in placement. This is required for overriding the location priority when requesting a placement
});
