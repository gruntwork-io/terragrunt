package runnerpool

type entry struct {
	task          *Task
	state         Status
	blockedBy     []*entry // parents
	dependents    []*entry // children (reverse edge)
	remainingDeps int      // countdown until 0 ➔ ready
	result        Result
}
