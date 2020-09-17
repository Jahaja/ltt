package ltt

import (
	"context"
	"strings"
)

type TaskFunc func(context.Context) error
type TaskSelectionStrategyType int

type taskContextKeyType int

var taskContextKey taskContextKeyType

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
	st := NewTask(name, t, opts)
	st.RunFunc = f

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

func TaskFromContext(ctx context.Context) *Task {
	t, _ := ctx.Value(taskContextKey).(*Task)
	return t
}

func NewTaskContext(ctx context.Context, t *Task) context.Context {
	return context.WithValue(ctx, taskContextKey, t)
}
