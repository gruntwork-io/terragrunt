import { S3Client, ListObjectsV2Command, GetObjectCommand } from '@aws-sdk/client-s3';
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { DynamoDBDocumentClient, GetCommand, UpdateCommand, ScanCommand } from '@aws-sdk/lib-dynamodb';
import { getSignedUrl } from '@aws-sdk/s3-request-presigner';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import Handlebars from 'handlebars';

const s3Client = new S3Client({
  maxAttempts: 3,
  requestHandler: {
    keepAlive: true
  }
});
const dynamoClient = new DynamoDBClient({
  maxAttempts: 3,
  requestHandler: {
    keepAlive: true
  }
});
const dynamodb = DynamoDBDocumentClient.from(dynamoClient);

// Get the directory path for ES modules
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Load static files
const templateHtml = readFileSync(join(__dirname, 'template.html'), 'utf8');
const stylesCss = readFileSync(join(__dirname, 'styles.css'), 'utf8');
const scriptJs = readFileSync(join(__dirname, 'script.js'), 'utf8');

// Compile Handlebars template
const template = Handlebars.compile(templateHtml);

// Server-side cache for presigned URLs
const presignedUrlCache = new Map();

// Server-side cache for S3 list response
const s3ListCache = {
  data: null,
  lastUpdated: 0,
  ttl: 10 * 1000 // 10 seconds
};

// Cache cleanup interval (every 5 minutes)
const CACHE_CLEANUP_INTERVAL = 5 * 60 * 1000;

// Initialize cache cleanup
setInterval(cleanupExpiredCache, CACHE_CLEANUP_INTERVAL);

// Function to clean up expired cache entries
function cleanupExpiredCache() {
  const now = Date.now();

  // Clean up presigned URL cache
  for (const [key, cacheEntry] of presignedUrlCache.entries()) {
    if (now > cacheEntry.expiresAt) {
      presignedUrlCache.delete(key);
    }
  }

  // Clean up S3 list cache if expired
  if (s3ListCache.data && (now - s3ListCache.lastUpdated) > s3ListCache.ttl) {
    s3ListCache.data = null;
    s3ListCache.lastUpdated = 0;
  }
}

// Function to get or generate presigned URL with caching
async function getCachedPresignedUrl(bucketName, imageKey) {
  const cacheKey = `${bucketName}:${imageKey}`;
  const now = Date.now();

  // Check if we have a valid cached URL
  const cached = presignedUrlCache.get(cacheKey);
  if (cached && now < cached.expiresAt) {
    return cached.url;
  }

  // Generate new presigned URL
  const getObjectCommand = new GetObjectCommand({
    Bucket: bucketName,
    Key: imageKey
  });

  const presignedUrl = await getSignedUrl(s3Client, getObjectCommand, { expiresIn: 3600 });

  // Cache the URL with expiration
  presignedUrlCache.set(cacheKey, {
    url: presignedUrl,
    expiresAt: now + (3600 * 1000) // 1 hour from now
  });

  return presignedUrl;
}

// Function to get cached S3 list data
async function getCachedS3List(bucketName) {
  const now = Date.now();

  // Check if we have valid cached data
  if (s3ListCache.data && (now - s3ListCache.lastUpdated) < s3ListCache.ttl) {
    console.log('Using cached S3 list data');
    return s3ListCache.data;
  }

  // Fetch fresh data from S3
  console.log('Fetching fresh S3 list data');
  const listCommand = new ListObjectsV2Command({
    Bucket: bucketName,
    MaxKeys: 100
  });
  const listResponse = await s3Client.send(listCommand);

  // Cache the response if we got data
  if (listResponse.Contents && listResponse.Contents.length > 0) {
    s3ListCache.data = listResponse;
    s3ListCache.lastUpdated = now;
    console.log(`Cached S3 list with ${listResponse.Contents.length} objects`);
  }

  return listResponse;
}

export async function handler(event) {
  const bucketName = process.env.S3_BUCKET_NAME;
  const tableName = process.env.DYNAMODB_TABLE_NAME;

  try {
    // Parse the request - Lambda function URLs have a different event structure
    const method = event.requestContext?.http?.method || event.httpMethod;
    const path = event.rawPath || event.path || '/';

    console.log('Request:', { method, path, event });

    // Serve static files
    if (method === 'GET' && path === '/styles.css') {
      return {
        statusCode: 200,
        headers: {
          'Content-Type': 'text/css',
          'Cache-Control': 'public, max-age=3600'
        },
        body: stylesCss
      };
    }

    if (method === 'GET' && path === '/script.js') {
      return {
        statusCode: 200,
        headers: {
          'Content-Type': 'application/javascript',
          'Cache-Control': 'public, max-age=3600'
        },
        body: scriptJs
      };
    }

    // Serve images with caching headers
    if (method === 'GET' && path.startsWith('/image/')) {
      const imageKey = path.substring(7); // Remove '/image/' prefix

      try {
        const presignedUrl = await getCachedPresignedUrl(bucketName, imageKey);

        return {
          statusCode: 302, // Redirect to presigned URL
          headers: {
            'Location': presignedUrl,
            'Cache-Control': 'public, max-age=3600, immutable'
          }
        };
      } catch (error) {
        console.error('Error generating presigned URL:', error);
        return {
          statusCode: 404,
          headers: {
            'Content-Type': 'text/html'
          },
          body: '<h1>Image not found</h1>'
        };
      }
    }

    // Main page - serve HTML
    if (method === 'GET' && (path === '/' || path === '')) {
      // Get all images from S3 (with caching)
      const listResponse = await getCachedS3List(bucketName);

      // Get all vote counts from DynamoDB
      const scanCommand = new ScanCommand({
        TableName: tableName
      });
      const votesResponse = await dynamodb.send(scanCommand);
      const votesMap = {};
      if (votesResponse.Items) {
        votesResponse.Items.forEach(item => {
          votesMap[item.image_id] = item.votes || 0;
        });
      }

      // Filter and prepare image data first
      const imageObjects = (listResponse.Contents || []).filter(obj =>
        obj.Key && obj.Key.match(/\.(jpg|jpeg|png|gif|webp)$/i)
      );

      // Generate presigned URLs in parallel for better performance
      const imagesWithUrls = await Promise.all(
        imageObjects.map(async (obj) => {
          const presignedUrl = await getCachedPresignedUrl(bucketName, obj.Key);
          return {
            key: obj.Key,
            keyId: obj.Key.replace(/[^a-zA-Z0-9]/g, '-'),
            url: presignedUrl,
            votes: votesMap[obj.Key] || 0
          };
        })
      );

      // Sort images by vote count (highest votes first)
      imagesWithUrls.sort((a, b) => b.votes - a.votes);

      // Render the template with data
      const html = template({
        images: imagesWithUrls
      });

      return {
        statusCode: 200,
        headers: {
          'Content-Type': 'text/html',
          'Cache-Control': 'public, max-age=60, s-maxage=300'
        },
        body: html
      };
    }

    // API endpoint for voting
    if (method === 'POST' && path.startsWith('/vote/')) {
      const parts = path.split('/');
      const imageId = parts[2];
      const voteType = parts[3]; // 'up' or 'down'

      const voteIncrement = voteType === 'up' ? 1 : -1;

      // Update vote count in DynamoDB
      const updateCommand = new UpdateCommand({
        TableName: tableName,
        Key: { image_id: imageId },
        UpdateExpression: 'SET votes = if_not_exists(votes, :zero) + :inc',
        ExpressionAttributeValues: {
          ':inc': voteIncrement,
          ':zero': 0
        },
        ReturnValues: 'UPDATED_NEW'
      });
      const result = await dynamodb.send(updateCommand);

              return {
          statusCode: 200,
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            message: 'Vote recorded successfully',
            image_id: imageId,
            new_vote_count: result.Attributes.votes
          })
        };
    }

    // API endpoint to get current vote counts
    if (method === 'GET' && path === '/api/votes') {
      const scanCommand = new ScanCommand({
        TableName: tableName
      });
      const votes = await dynamodb.send(scanCommand);

              return {
          statusCode: 200,
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            votes: votes.Items || []
          })
        };
    }

    // For any other GET request, serve the main page (helpful for debugging)
    if (method === 'GET') {
      console.log('Serving main page for path:', path);

      // Get all images from S3 (with caching)
      const listResponse = await getCachedS3List(bucketName);

      // Get all vote counts from DynamoDB
      const scanCommand = new ScanCommand({
        TableName: tableName
      });
      const votesResponse = await dynamodb.send(scanCommand);
      const votesMap = {};
      if (votesResponse.Items) {
        votesResponse.Items.forEach(item => {
          votesMap[item.image_id] = item.votes || 0;
        });
      }

      // Filter and prepare image data efficiently
      const imageObjects = (listResponse.Contents || []).filter(obj =>
        obj.Key && obj.Key.match(/\.(jpg|jpeg|png|gif|webp)$/i)
      );

      const imagesWithUrls = imageObjects.map(obj => ({
        key: obj.Key,
        keyId: obj.Key.replace(/[^a-zA-Z0-9]/g, '-'),
        votes: votesMap[obj.Key] || 0
      }));

      // Sort images by vote count (highest votes first)
      imagesWithUrls.sort((a, b) => b.votes - a.votes);

      // Render the template with data
      const html = template({
        images: imagesWithUrls
      });

      return {
        statusCode: 200,
        headers: {
          'Content-Type': 'text/html',
          'Cache-Control': 'public, max-age=60, s-maxage=300'
        },
        body: html
      };
    }

    // Default response for unknown endpoints
    return {
      statusCode: 404,
      headers: {
        'Content-Type': 'text/html'
      },
      body: `
        <html>
          <head><title>404 - Not Found</title></head>
          <body>
            <h1>404 - Page Not Found</h1>
            <p>The requested page could not be found.</p>
            <p>Method: ${method}, Path: ${path}</p>
            <a href="/">Go back to the main page</a>
          </body>
        </html>
      `
    };

  } catch (error) {
    console.error('Error:', error);

    return {
      statusCode: 500,
      headers: {
        'Content-Type': 'text/html'
      },
      body: `
        <html>
          <head><title>500 - Server Error</title></head>
          <body>
            <h1>500 - Internal Server Error</h1>
            <p>An error occurred while processing your request.</p>
            <a href="/">Go back to the main page</a>
          </body>
        </html>
      `
    };
  }
}
