package agent

import (
	"fmt"
	"time"
)

const TimeFormat = "15:04"

type ActivityLog struct {
	// at first is a date. current time is not provided
	Date    time.Time
	Content string
}

type ActivityRepo interface {
	Log(ActivityRecord) error
	GetActivity(time.Time) (string, error)
	GetRange(time.Time, time.Time) ([]ActivityLog, error)
}

type ConsolidatorRepo interface {
	GetMemory(string) (string, error)
}

type MemoryIndexer interface {
	MemoryIndex() (map[string]string, error)
}

type ActivityRecord struct {
	Stamp   time.Time
	Content string
}

func NewRecord(content string) ActivityRecord {
	return ActivityRecord{
		Stamp:   time.Now(),
		Content: content,
	}
}

func (r ActivityRecord) String() string {
	timeHeader := time.Now().Format(TimeFormat)
	return fmt.Sprintf("## %s\n%s\n\n", timeHeader, r.Content)
}
