package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

const teamStatsQuery = `
WITH team_tasks AS (
    SELECT id, status, assignee_id, created_at, closed_at
    FROM tasks
    WHERE team_id = ?
),
status_counts AS (
    SELECT status, COUNT(*) AS cnt
    FROM team_tasks
    GROUP BY status
),
top_assignees AS (
    SELECT u.id, u.name, u.email, COUNT(*) AS closed,
           ROW_NUMBER() OVER (ORDER BY COUNT(*) DESC, u.id) AS pos
    FROM team_tasks t
    JOIN users u ON u.id = t.assignee_id
    WHERE t.status = 'done' AND t.closed_at >= ? - INTERVAL 30 DAY
    GROUP BY u.id, u.name, u.email
),
close_time AS (
    SELECT AVG(TIMESTAMPDIFF(SECOND, created_at, closed_at)) AS avg_seconds
    FROM team_tasks
    WHERE closed_at IS NOT NULL
),
comment_count AS (
    SELECT COUNT(c.id) AS total
    FROM task_comments c
    JOIN team_tasks t ON t.id = c.task_id
)
SELECT 'status' AS metric, status AS label, NULL AS user_id, NULL AS email, cnt AS value, 0 AS pos FROM status_counts
UNION ALL
SELECT 'top_assignee', name, id, email, closed, pos FROM top_assignees WHERE pos <= 3
UNION ALL
SELECT 'avg_close_seconds', NULL, NULL, NULL, avg_seconds, 0 FROM close_time
UNION ALL
SELECT 'comments', NULL, NULL, NULL, total, 0 FROM comment_count
ORDER BY metric, pos`

type Stats struct {
	db *sql.DB
}

func NewStats(db *sql.DB) *Stats {
	return &Stats{db: db}
}

type statsRow struct {
	metric string
	label  sql.NullString
	userID sql.NullInt64
	email  sql.NullString
	value  sql.NullFloat64
}

func (r *Stats) TeamStats(ctx context.Context, teamID int64, now time.Time) (domain.TeamStats, error) {
	rows, err := queryAll(ctx, conn(ctx, r.db), scanStatsRow, teamStatsQuery, teamID, now)
	if err != nil {
		return domain.TeamStats{}, fmt.Errorf("select team stats: %w", err)
	}

	stats := domain.TeamStats{TeamID: teamID, TasksByStatus: make(map[domain.Status]int, len(domain.Statuses))}
	for _, s := range domain.Statuses {
		stats.TasksByStatus[s] = 0
	}
	for _, row := range rows {
		switch row.metric {
		case "status":
			stats.TasksByStatus[domain.Status(row.label.String)] = int(row.value.Float64)
		case "top_assignee":
			stats.TopAssignees = append(stats.TopAssignees, domain.AssigneeStats{
				UserID:      row.userID.Int64,
				Name:        row.label.String,
				Email:       row.email.String,
				ClosedTasks: int(row.value.Float64),
			})
		case "avg_close_seconds":
			if row.value.Valid {
				avg := time.Duration(row.value.Float64 * float64(time.Second))
				stats.AvgCloseTime = &avg
			}
		case "comments":
			stats.CommentsTotal = int(row.value.Float64)
		}
	}
	return stats, nil
}

func scanStatsRow(s scanner) (statsRow, error) {
	var (
		row statsRow
		pos int
	)
	err := s.Scan(&row.metric, &row.label, &row.userID, &row.email, &row.value, &pos)
	return row, err
}
