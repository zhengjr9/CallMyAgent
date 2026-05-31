package main

import (
	"fmt"
	"sync"
	"time"
)

type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	seq   int
}

func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks: make(map[string]*Task),
	}
}

func (s *TaskStore) Create(title, description, gitRepo, gitBranch, engine string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	now := time.Now().UTC().Format(time.RFC3339)
	if engine == "" {
		engine = "claude"
	}
	t := &Task{
		ID:          fmt.Sprintf("task-%d", s.seq),
		Title:       title,
		Description:  description,
		GitRepo:     gitRepo,
		GitBranch:   gitBranch,
		Engine:      engine,
		Status:      "chatting",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.tasks[t.ID] = t
	return t
}

func (s *TaskStore) Get(id string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[id]
}

func (s *TaskStore) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t)
	}
	return result
}

func (s *TaskStore) Update(t *Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.tasks[t.ID] = t
}

func (s *TaskStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
}
