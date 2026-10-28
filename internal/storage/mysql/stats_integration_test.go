//go:build integration

package mysql_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/lapp-coder/task-manager-service/internal/config"
	"github.com/lapp-coder/task-manager-service/internal/domain"
	"github.com/lapp-coder/task-manager-service/internal/storage/mysql"
	"github.com/lapp-coder/task-manager-service/migrations"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestTeamStats(t *testing.T) {
	ctx := context.Background()
	db := startMySQL(ctx, t)
	s := newSeeder(ctx, t, db)

	alice := s.user("alice@example.com")
	bob := s.user("bob@example.com")
	carol := s.user("carol@example.com")
	dave := s.user("dave@example.com")
	eve := s.user("eve@example.com")

	team := s.team(alice, bob, carol, dave)
	otherTeam := s.team(eve)

	t1 := s.closedTask(team, bob, now.Add(-50*time.Hour), now.Add(-2*time.Hour))
	s.closedTask(team, bob, now.AddDate(0, 0, -10), now.AddDate(0, 0, -9))
	s.closedTask(team, carol, now.AddDate(0, 0, -38), now.AddDate(0, 0, -35))
	s.closedTask(team, carol, now.AddDate(0, 0, -5), now.AddDate(0, 0, -4))
	s.closedTask(team, dave, now.AddDate(0, 0, -3), now.AddDate(0, 0, -2))
	s.closedTask(team, alice, now.AddDate(0, 0, -1), now.Add(-12*time.Hour))
	t7 := s.openTask(team, bob, domain.StatusInProgress)
	s.openTask(team, 0, domain.StatusTodo)
	s.openTask(team, dave, domain.StatusTodo)
	s.comments(t1, alice, 2)
	s.comments(t7, bob, 1)

	foreign := s.closedTask(otherTeam, eve, now.Add(-3*time.Hour), now.Add(-time.Hour))
	s.comments(foreign, eve, 5)

	stats, err := mysql.NewStats(db).TeamStats(ctx, team, now)
	require.NoError(t, err)

	avg := 34 * time.Hour
	require.Equal(t, domain.TeamStats{
		TeamID: team,
		TasksByStatus: map[domain.Status]int{
			domain.StatusTodo:       2,
			domain.StatusInProgress: 1,
			domain.StatusDone:       6,
		},
		TopAssignees: []domain.AssigneeStats{
			{UserID: bob, Name: "bob@example.com", Email: "bob@example.com", ClosedTasks: 2},
			{UserID: alice, Name: "alice@example.com", Email: "alice@example.com", ClosedTasks: 1},
			{UserID: carol, Name: "carol@example.com", Email: "carol@example.com", ClosedTasks: 1},
		},
		AvgCloseTime:  &avg,
		CommentsTotal: 3,
	}, stats)

	empty := s.team(alice)
	stats, err = mysql.NewStats(db).TeamStats(ctx, empty, now)
	require.NoError(t, err)
	require.Equal(t, domain.TeamStats{
		TeamID: empty,
		TasksByStatus: map[domain.Status]int{
			domain.StatusTodo:       0,
			domain.StatusInProgress: 0,
			domain.StatusDone:       0,
		},
	}, stats)
}

func startMySQL(ctx context.Context, t *testing.T) *sql.DB {
	t.Helper()

	ctr, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("tasks"),
		tcmysql.WithUsername("app"),
		tcmysql.WithPassword("secret"),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	host, err := ctr.Host(ctx)
	require.NoError(t, err)
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	require.NoError(t, err)

	db, err := mysql.Open(ctx, config.MySQL{
		Host:            host,
		Port:            int(port.Num()),
		User:            "app",
		Password:        "secret",
		Database:        "tasks",
		MaxOpenConns:    4,
		MaxIdleConns:    4,
		ConnMaxLifetime: time.Minute,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	provider, err := goose.NewProvider(goose.DialectMySQL, db, migrations.FS)
	require.NoError(t, err)
	_, err = provider.Up(ctx)
	require.NoError(t, err)
	return db
}

type seeder struct {
	ctx   context.Context
	t     *testing.T
	users *mysql.Users
	teams *mysql.Teams
	tasks *mysql.Tasks
}

func newSeeder(ctx context.Context, t *testing.T, db *sql.DB) *seeder {
	return &seeder{ctx: ctx, t: t, users: mysql.NewUsers(db), teams: mysql.NewTeams(db), tasks: mysql.NewTasks(db)}
}

func (s *seeder) user(email string) int64 {
	id, err := s.users.Create(s.ctx, domain.User{Email: email, Name: email, PasswordHash: "x", CreatedAt: now})
	require.NoError(s.t, err)
	return id
}

func (s *seeder) team(owner int64, members ...int64) int64 {
	id, err := s.teams.Create(s.ctx, domain.Team{Name: "team", CreatedBy: owner, CreatedAt: now})
	require.NoError(s.t, err)
	require.NoError(s.t, s.teams.AddMember(s.ctx, domain.Member{TeamID: id, UserID: owner, Role: domain.RoleOwner, JoinedAt: now}))
	for _, m := range members {
		require.NoError(s.t, s.teams.AddMember(s.ctx, domain.Member{TeamID: id, UserID: m, Role: domain.RoleMember, JoinedAt: now}))
	}
	return id
}

func (s *seeder) closedTask(teamID, assigneeID int64, createdAt, closedAt time.Time) int64 {
	return s.task(domain.Task{
		TeamID:     teamID,
		Title:      "closed",
		Status:     domain.StatusDone,
		AssigneeID: assigneeID,
		CreatedAt:  createdAt,
		UpdatedAt:  closedAt,
		ClosedAt:   closedAt,
	})
}

func (s *seeder) openTask(teamID, assigneeID int64, status domain.Status) int64 {
	return s.task(domain.Task{
		TeamID:     teamID,
		Title:      "open",
		Status:     status,
		AssigneeID: assigneeID,
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now.Add(-time.Hour),
	})
}

func (s *seeder) task(t domain.Task) int64 {
	members, err := s.teams.Members(s.ctx, t.TeamID)
	require.NoError(s.t, err)
	t.CreatedBy = members[0].UserID
	t.Version = 1
	id, err := s.tasks.Create(s.ctx, t)
	require.NoError(s.t, err)
	return id
}

func (s *seeder) comments(taskID, userID int64, n int) {
	for range n {
		_, err := s.tasks.AddComment(s.ctx, domain.Comment{TaskID: taskID, UserID: userID, Content: "c", CreatedAt: now})
		require.NoError(s.t, err)
	}
}
