package service_test

import (
	"context"
	"sync"
	"time"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fakeTeams struct {
	mu      sync.Mutex
	members map[[2]int64]domain.Member
}

func newFakeTeams(members ...domain.Member) *fakeTeams {
	f := &fakeTeams{members: make(map[[2]int64]domain.Member)}
	for _, m := range members {
		f.members[[2]int64{m.TeamID, m.UserID}] = m
	}
	return f
}

func (f *fakeTeams) Member(_ context.Context, teamID, userID int64) (domain.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.members[[2]int64{teamID, userID}]
	if !ok {
		return domain.Member{}, domain.ErrMemberNotFound
	}
	return m, nil
}

func (f *fakeTeams) Create(context.Context, domain.Team) (int64, error) { return 1, nil }

func (f *fakeTeams) AddMember(_ context.Context, m domain.Member) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := [2]int64{m.TeamID, m.UserID}
	if _, ok := f.members[key]; ok {
		return domain.ErrAlreadyMember
	}
	f.members[key] = m
	return nil
}

func (f *fakeTeams) UpdateRole(_ context.Context, m domain.Member) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[[2]int64{m.TeamID, m.UserID}] = m
	return nil
}

func (f *fakeTeams) ListByUser(context.Context, int64) ([]domain.Membership, error) { return nil, nil }

func (f *fakeTeams) Members(context.Context, int64) ([]domain.TeamMember, error) { return nil, nil }

type fakeUsers map[string]domain.User

func (f fakeUsers) ByEmail(_ context.Context, email string) (domain.User, error) {
	u, ok := f[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

type fakeStats struct{}

func (fakeStats) TeamStats(_ context.Context, teamID int64, _ time.Time) (domain.TeamStats, error) {
	return domain.TeamStats{TeamID: teamID}, nil
}

type fakeTasks struct {
	mu      sync.Mutex
	tasks   map[int64]domain.Task
	history []domain.HistoryEntry
	nextID  int64
}

func newFakeTasks(tasks ...domain.Task) *fakeTasks {
	f := &fakeTasks{tasks: make(map[int64]domain.Task), nextID: 1000}
	for _, t := range tasks {
		f.tasks[t.ID] = t
	}
	return f
}

func (f *fakeTasks) Create(_ context.Context, t domain.Task) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	t.ID = f.nextID
	f.tasks[t.ID] = t
	return t.ID, nil
}

func (f *fakeTasks) ByID(_ context.Context, id int64) (domain.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return domain.Task{}, domain.ErrTaskNotFound
	}
	return t, nil
}

func (f *fakeTasks) ByIDForUpdate(ctx context.Context, id int64) (domain.Task, error) {
	return f.ByID(ctx, id)
}

func (f *fakeTasks) Update(_ context.Context, t domain.Task, prevVersion int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tasks[t.ID].Version != prevVersion {
		return domain.ErrVersionConflict
	}
	f.tasks[t.ID] = t
	return nil
}

func (f *fakeTasks) List(_ context.Context, filter domain.TaskFilter) (domain.Page[domain.Task], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var page domain.Page[domain.Task]
	for _, t := range f.tasks {
		if t.TeamID == filter.TeamID {
			page.Items = append(page.Items, t)
		}
	}
	return page, nil
}

func (f *fakeTasks) AddHistory(_ context.Context, h domain.HistoryEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history = append(f.history, h)
	return nil
}

func (f *fakeTasks) History(context.Context, int64, domain.PageRequest) (domain.Page[domain.HistoryEntry], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domain.Page[domain.HistoryEntry]{Items: f.history}, nil
}

func (f *fakeTasks) AddComment(context.Context, domain.Comment) (int64, error) { return 1, nil }

func (f *fakeTasks) Comments(context.Context, int64, domain.PageRequest) (domain.Page[domain.Comment], error) {
	return domain.Page[domain.Comment]{}, nil
}

func (f *fakeTasks) recorded() []domain.HistoryEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.HistoryEntry(nil), f.history...)
}

type fakeCache struct {
	mu          sync.Mutex
	invalidated []int64
}

func (c *fakeCache) Tasks(
	ctx context.Context,
	_ domain.TaskFilter,
	load func(context.Context) (domain.Page[domain.Task], error),
) (domain.Page[domain.Task], error) {
	return load(ctx)
}

func (c *fakeCache) Invalidate(_ context.Context, teamID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidated = append(c.invalidated, teamID)
}

func (c *fakeCache) invalidations() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.invalidated...)
}
