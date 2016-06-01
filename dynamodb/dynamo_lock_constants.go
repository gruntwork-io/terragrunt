package dynamodb

import "time"

const ATTR_STATE_FILE_ID = "StateFileId"
const ATTR_USERNAME = "Username"
const ATTR_IP = "Ip"
const ATTR_CREATION_DATE = "CreationDate"

const SLEEP_BETWEEN_TABLE_STATUS_CHECKS = 10 * time.Second
const SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS = 10 * time.Second

const MAX_RETRIES_WAITING_FOR_TABLE_TO_BE_ACTIVE = 30
