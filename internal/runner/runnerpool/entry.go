package runnerpool

type entry struct {
	task          *Task
	state         Status
	remainingDeps int
	dependents    []*entry
	result        Result
}
