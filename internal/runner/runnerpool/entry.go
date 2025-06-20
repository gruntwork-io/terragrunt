package runnerpool

type entry struct {
	task      *Task
	state     Status
	blockedBy []*entry
	result    Result
}
