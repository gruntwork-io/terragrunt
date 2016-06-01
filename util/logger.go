package util

import (
	"os"
	"log"
)

// A simple logger we can use to get consistent log formatting through out the app
var Logger = log.New(os.Stdout, "[terragrunt] ", log.LstdFlags)
