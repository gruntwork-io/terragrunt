package a

import "time"

// nonTestFileSleep exists to confirm that the analyzer ignores non *_test.go
// files entirely. If this file were scanned, the call below would be flagged.
func nonTestFileSleep() {
	time.Sleep(0)
}
