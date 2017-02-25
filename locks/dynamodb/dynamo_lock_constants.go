package dynamodb

import "time"

// The names of attributes we use in the DynamoDB lock table
const ATTR_STATE_FILE_ID = "StateFileId"
const ATTR_USERNAME = "Username"
const ATTR_IP = "Ip"
const ATTR_CREATION_DATE = "CreationDate"

// Default is to retry for up to 5 minutes
const MAX_RETRIES_WAITING_FOR_TABLE_TO_BE_ACTIVE = 30
const SLEEP_BETWEEN_TABLE_STATUS_CHECKS = 10 * time.Second

// Default is to retry for up to 1 hour
const DEFAULT_MAX_RETRIES_WAITING_FOR_LOCK = 360
const SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS = 10 * time.Second

const DEFAULT_TABLE_NAME = "terragrunt_locks"
const DEFAULT_AWS_REGION = "us-east-1"

const DEFAULT_READ_CAPACITY_UNITS = 1
const DEFAULT_WRITE_CAPACITY_UNITS = 1
