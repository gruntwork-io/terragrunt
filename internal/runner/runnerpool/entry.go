package runnerpool

type entry struct {
	result        Result
	task          *Task
	dependents    []*entry
	state         Status
	remainingDeps int
}
