package loadtest

import (
	"context"
	"log"
	"math/rand"
	"time"
)

type userContextKeyType int

var userContextKey userContextKeyType

type User interface {
	ID() int64
	SetID(id int64)

	SetContext(ctx context.Context)
	Context() context.Context

	Spawn()
	Tick()
	Sleep()
}

type DefaultUser struct {
	id           int64
	ctx          context.Context
	task         *Task
	subtaskIndex int
}

func NewDefaultUser(task *Task) *DefaultUser {
	du := &DefaultUser{
		task:         task,
		subtaskIndex: -1,
	}

	return du
}

func UserFromContext(ctx context.Context) User {
	if u, ok := ctx.Value(userContextKey).(User); ok {
		return u
	}

	return nil
}

func NewUserContext(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

func (du *DefaultUser) SetID(id int64) {
	du.id = id
}

func (du *DefaultUser) ID() int64 {
	return du.id
}

func (du *DefaultUser) SetContext(ctx context.Context) {
	du.ctx = ctx
}

func (du *DefaultUser) Context() context.Context {
	return du.ctx
}

func (du *DefaultUser) Spawn() {
	lt := FromContext(du.Context())
	if lt.DefaultUserSpawn != nil {
		lt.DefaultUserSpawn(du)
	}
}

func (du *DefaultUser) Tick() {
	const poolStepOut = -1

	var next *Task
	if du.task.Options.SelectionStrategy == TaskSelectionStrategyRandom {
		// TOOD(jhamren): infinite loop check or validate loop-tree on startup
		if du.task.Parent != nil && len(du.task.SubTasks) == 0 {
			du.task = du.task.Parent
			du.Tick()
			return
		}

		// Create a pool of the subtasks and their wight and pick a random index
		// from the pool after shuffling it
		pool := make([]int, len(du.task.SubTasks))

		for i, t := range du.task.SubTasks {
			pool = append(pool, i)

			// Add the same index again to the pool according to its weight
			for j := 0; j < t.Options.Weight; j++ {
				pool = append(pool, i)
			}
		}

		// Make sure that the task sometimes steps out of their subtasks
		if du.task.Parent != nil {
			pool = append(pool, poolStepOut)
			for i := 0; i < du.task.Options.StepOutWeight; i++ {
				pool = append(pool, poolStepOut)
			}
		}

		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(pool), func(i, j int) {
			pool[i], pool[j] = pool[j], pool[i]
		})

		ix := pool[rand.Intn(len(pool))]
		if ix == poolStepOut {
			du.task = du.task.Parent
			du.Tick()
			return
		} else {
			next = du.task.SubTasks[ix]
		}
	} else if du.task.Options.SelectionStrategy == TaskSelectionStrategyInOrder {
		du.subtaskIndex++

		if du.subtaskIndex >= len(du.task.SubTasks) {
			du.subtaskIndex = 0

			// all tasks have been run once, step out to parent task if there's one
			// otherwise, start over on 0
			if du.task.Parent != nil {
				du.task = du.task.Parent
				du.Tick()
				return
			}
		}

		next = du.task.SubTasks[du.subtaskIndex]
	} else {
		log.Fatal("failed to select a task")
	}

	du.task = next
	if du.task.RunFunc != nil {
		// Pass on the current task in the context
		du.SetContext(NewTaskContext(du.Context(), next))

		start := time.Now()
		err := du.task.RunFunc(du.Context())

		duration := time.Now().Sub(start)
		lt := FromContext(du.Context())
		lt.TaskRunChan <- &TaskRun{
			Task:     du.task,
			Duration: duration,
			Error:    err,
		}
	}
}

func (du *DefaultUser) Sleep() {
	lt := FromContext(du.Context())
	if lt.DefaultUserSleep != nil {
		lt.DefaultUserSleep(du)
		return
	}

	rand.Seed(time.Now().UnixNano())

	sleepTime := lt.Config.MinSleepTime
	sleepTime += rand.Intn(lt.Config.MaxSleepTime)

	log.Printf("DefaultUser(%d): sleeping for %d seconds\n", du.ID(), sleepTime)
	time.Sleep(time.Second * time.Duration(sleepTime))
}
