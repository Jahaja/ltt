package loadtest

import (
	"context"
)

type TaskFunc func(context.Context) error
type TaskSelectionStrategyType int

const (
	TaskSelectionStrategyRandom TaskSelectionStrategyType = iota
	TaskSelectionStrategyInOrder
)

type TaskOptions struct {
	SelectionStrategy TaskSelectionStrategyType
	StepOutWeight     int
	Weight            int
}

type Task struct {
	Name     string
	Parent   *Task
	SubTasks []*Task
	RunFunc  TaskFunc
	Options  TaskOptions
}

// Add and returns an empty section Task and passes it to the callback
// The TaskOptions will be set on the new section task.
func (t *Task) AddSection(name string, setup func(*Task), opts TaskOptions) *Task {
	st := t.AddTask(name, nil, opts)
	setup(st)
	return st
}

// Add a subtask
func (t *Task) AddTask(name string, f TaskFunc, opts TaskOptions) *Task {
	st := NewTask(name, t, opts)
	st.RunFunc = f

	t.SubTasks = append(t.SubTasks, st)
	return st
}

func (t *Task) Next() *Task {
	return nil
}

func NewEntryTask(name string, opts TaskOptions) *Task {
	return NewTask(name, nil, opts)
}

func NewTask(name string, parent *Task, opts TaskOptions) *Task {
	return &Task{
		Name:    name,
		Parent:  parent,
		Options: opts,
	}
}
