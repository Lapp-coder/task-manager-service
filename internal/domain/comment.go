package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const maxCommentLen = 5000

type Comment struct {
	ID        int64
	TaskID    int64
	UserID    int64
	Content   string
	CreatedAt time.Time
}

func NewComment(taskID, userID int64, content string, now time.Time) (Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > maxCommentLen {
		return Comment{}, fmt.Errorf("%w: content must be 1 to %d characters long", ErrInvalidInput, maxCommentLen)
	}
	return Comment{TaskID: taskID, UserID: userID, Content: content, CreatedAt: now}, nil
}
