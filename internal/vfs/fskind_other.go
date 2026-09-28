//go:build !linux && !darwin && !windows

package vfs

// detectFSKind has no probe on platforms terragrunt does not ship
// tuning for. It reports [FSUnknown] rather than [FSUnprobed] because
// no other path would answer either: a caller that climbs ancestors
// looking for an answer would only burn syscalls on the way up.
func detectFSKind(string) FSKind { return FSUnknown }
