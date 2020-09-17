package ltt

import (
	"context"
	"strings"
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

// Adds and returns an empty section Task and passes it to the callback
// The TaskOptions will be set on the new section task.
func (t *Task) AddSection(name string, setup func(*Task), opts TaskOptions) *Task {
	st := t.AddSubTask(name, nil, opts)
	setup(st)
	return st
}

func (t *Task) AddSubTask(name string, f TaskFunc, opts TaskOptions) *Task {
	st := NewTask(name, t, f, opts)

	t.SubTasks = append(t.SubTasks, st)
	return st
}

func (t *Task) FullName() string {
	if t.Parent == nil {
		return t.Name
	}

	parentNames := []string{}
	p := t.Parent
	for p != nil {
		parentNames = append(parentNames, p.Name)
		p = p.Parent
	}

	sb := strings.Builder{}
	for i := len(parentNames) - 1; i >= 0; i-- {
		sb.WriteString(parentNames[i])
		sb.WriteString(" / ")
	}
	sb.WriteString(t.Name)

	return sb.String()
}

// The entry task is the first task each User run, and only once
func NewEntryTask(name string, f TaskFunc, opts TaskOptions) *Task {
	return NewTask(name, nil, f, opts)
}

func NewTask(name string, parent *Task, f TaskFunc, opts TaskOptions) *Task {
	return &Task{
		Name:    name,
		Parent:  parent,
		RunFunc: f,
		Options: opts,
	}
}
