package files

import (
	"ard/internal/agent"
	"ard/internal/types"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var _ agent.ActivityRepo = (*ActivityFiles)(nil)

const ActivityFolder = "activity"

const dayDuration = 24 * time.Hour

type ActivityFiles struct {
	storage FileStorage
}

func NewActivityFiles(storage FileStorage) *ActivityFiles {
	return &ActivityFiles{
		storage: storage,
	}
}

func (a *ActivityFiles) Log(r agent.ActivityRecord) error {
	data := []byte(r.String())

	p := resolveActivityFilePath(time.Now())

	if err := a.storage.MkdirAll(filepath.Dir(p), ModeDirPerm); err != nil {
		return err
	}

	f, err := a.storage.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, ModeFilePerm)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func (a *ActivityFiles) GetActivity(date time.Time) (string, error) {
	data, err := a.storage.ReadFile(resolveActivityFilePath(date))
	if err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return "", types.ErrIsNotExist
		}
		return "", err
	}
	return string(data), nil
}

func (a *ActivityFiles) GetRange(
	from time.Time,
	to time.Time,
) ([]agent.ActivityLog, error) {
	_, err := a.storage.Stat(ActivityFolder)
	if err != nil {
		if _, ok := errors.AsType[*os.PathError](err); ok {
			return nil, types.ErrIsNotExist
		}
		return nil, err
	}

	logs := []agent.ActivityLog{}

	if to.IsZero() {
		to = time.Now()
	}

	for i := from; !i.After(to); i = i.AddDate(0, 0, 1) {
		p := resolveActivityFilePath(i)
		data, err := a.storage.ReadFile(p)
		if err != nil && !errors.Is(err, types.ErrIsNotExist) {
			return nil, err
		}
		if data != nil {
			logs = append(logs, agent.ActivityLog{
				Date:    i,
				Content: string(data),
			})
		}
	}

	return logs, nil
}

func resolveActivityFilePath(t time.Time) string {
	d := t.UTC().Truncate(dayDuration)
	return filepath.Join(
		ActivityFolder,
		d.Format("2006"),
		d.Format("01"),
		fmt.Sprintf("%s.md", d.Format("02")),
	)
}
