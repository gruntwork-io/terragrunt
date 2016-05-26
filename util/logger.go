package util

import (
	"os"
	"log"
)

var Logger = log.New(os.Stdout, "[terragrunt] ", log.LstdFlags)
