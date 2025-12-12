const { DynamoDBClient, PutItemCommand } = require('@aws-sdk/client-dynamodb');
const dynamodb = new DynamoDBClient();

exports.handler = async (event) => {
  console.log('SNS Event:', JSON.stringify(event, null, 2));
  
  for (const record of event.Records) {
    try {
      const message = JSON.parse(record.Sns.Message);
      const detail = message.detail;
      
      if (!detail || !detail.placementId) {
        console.error('Invalid event structure - missing detail or placementId:', message);
        continue;
      }
      
      const item = {
        placementId: { S: detail.placementId },
        timestamp: { S: new Date().toISOString() },
        type: { S: detail.type || 'unknown' },
        queueHomeRegion: { S: message.region || 'unknown' },
        rawEventData: { S: JSON.stringify(message) },
        ttl: { N: String(Math.floor(Date.now() / 1000) + (14 * 24 * 60 * 60)) }
      };
      
      if (detail.startTime) item.startTime = { S: detail.startTime };
      if (detail.endTime) item.endTime = { S: detail.endTime };
      
      // Add PlacementFulfilled specific data
      if (detail.type === 'PlacementFulfilled') {
        if (detail.port) item.port = { S: detail.port };
        if (detail.ipAddress) item.ipAddress = { S: detail.ipAddress };
        if (detail.dnsName) item.dnsName = { S: detail.dnsName };
        if (detail.gameSessionRegion) item.gameSessionRegion = { S: detail.gameSessionRegion };
        if (detail.gameSessionArn) item.gameSessionArn = { S: detail.gameSessionArn };
        if (detail.placedPlayerSessions) item.placedPlayerSessions = { S: JSON.stringify(detail.placedPlayerSessions) };
      }
      
      await dynamodb.send(new PutItemCommand({
        TableName: process.env.TABLE_NAME,
        Item: item
      }));
      
      console.log(`Successfully processed placement: ${detail.placementId}`);
      
    } catch (error) {
      console.error('Error processing record:', error);
      console.error('Failed record:', JSON.stringify(record, null, 2));
      // Continue processing other records instead of failing entire batch
    }
  }
  
  return { statusCode: 200 };
};
