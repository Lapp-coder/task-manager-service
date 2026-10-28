package domain

import "time"

type TeamStats struct {
	TeamID        int64
	TasksByStatus map[Status]int
	TopAssignees  []AssigneeStats
	AvgCloseTime  *time.Duration
	CommentsTotal int
}

type AssigneeStats struct {
	UserID      int64
	Name        string
	Email       string
	ClosedTasks int
}
