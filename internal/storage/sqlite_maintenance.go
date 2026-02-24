package storage

import (
	"context"
	"encoding/json"
	"time"
)

func (s *SQLiteStore) CreateMaintenanceWindow(ctx context.Context, mw *MaintenanceWindow) error {
	monitorIDs, _ := json.Marshal(mw.MonitorIDs)
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO maintenance_windows (name, monitor_ids, start_time, end_time, recurring, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		mw.Name, string(monitorIDs), formatTime(mw.StartTime), formatTime(mw.EndTime), mw.Recurring, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	mw.ID = id
	mw.CreatedAt = parseTime(now)
	mw.UpdatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) GetMaintenanceWindow(ctx context.Context, id int64) (*MaintenanceWindow, error) {
	var mw MaintenanceWindow
	var monitorIDsStr, startTime, endTime, createdAt, updatedAt string
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, name, monitor_ids, start_time, end_time, recurring, created_at, updated_at
		 FROM maintenance_windows WHERE id=?`, id).
		Scan(&mw.ID, &mw.Name, &monitorIDsStr, &startTime, &endTime, &mw.Recurring, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	mw.StartTime = parseTime(startTime)
	mw.EndTime = parseTime(endTime)
	mw.CreatedAt = parseTime(createdAt)
	mw.UpdatedAt = parseTime(updatedAt)
	json.Unmarshal([]byte(monitorIDsStr), &mw.MonitorIDs)
	return &mw, nil
}

func (s *SQLiteStore) ListMaintenanceWindows(ctx context.Context) ([]*MaintenanceWindow, error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT id, name, monitor_ids, start_time, end_time, recurring, created_at, updated_at
		 FROM maintenance_windows ORDER BY start_time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var windows []*MaintenanceWindow
	for rows.Next() {
		var mw MaintenanceWindow
		var monitorIDsStr, startTime, endTime, createdAt, updatedAt string
		if err := rows.Scan(&mw.ID, &mw.Name, &monitorIDsStr, &startTime, &endTime, &mw.Recurring, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		mw.StartTime = parseTime(startTime)
		mw.EndTime = parseTime(endTime)
		mw.CreatedAt = parseTime(createdAt)
		mw.UpdatedAt = parseTime(updatedAt)
		json.Unmarshal([]byte(monitorIDsStr), &mw.MonitorIDs)
		windows = append(windows, &mw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if windows == nil {
		windows = []*MaintenanceWindow{}
	}
	return windows, nil
}

func (s *SQLiteStore) UpdateMaintenanceWindow(ctx context.Context, mw *MaintenanceWindow) error {
	monitorIDs, _ := json.Marshal(mw.MonitorIDs)
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		`UPDATE maintenance_windows SET name=?, monitor_ids=?, start_time=?, end_time=?, recurring=?, updated_at=? WHERE id=?`,
		mw.Name, string(monitorIDs), formatTime(mw.StartTime), formatTime(mw.EndTime), mw.Recurring, now, mw.ID)
	return err
}

func (s *SQLiteStore) DeleteMaintenanceWindow(ctx context.Context, id int64) error {
	_, err := s.writeDB.ExecContext(ctx, "DELETE FROM maintenance_windows WHERE id=?", id)
	return err
}

func (s *SQLiteStore) IsMonitorInMaintenance(ctx context.Context, monitorID int64, at time.Time) (bool, error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT id, name, monitor_ids, start_time, end_time, recurring, created_at, updated_at
		 FROM maintenance_windows
		 WHERE recurring != '' OR end_time > ?`,
		formatTime(at))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var mw MaintenanceWindow
		var monitorIDsStr, startTime, endTime, createdAt, updatedAt string
		if err := rows.Scan(&mw.ID, &mw.Name, &monitorIDsStr, &startTime, &endTime, &mw.Recurring, &createdAt, &updatedAt); err != nil {
			return false, err
		}
		mw.StartTime = parseTime(startTime)
		mw.EndTime = parseTime(endTime)
		mw.CreatedAt = parseTime(createdAt)
		mw.UpdatedAt = parseTime(updatedAt)
		json.Unmarshal([]byte(monitorIDsStr), &mw.MonitorIDs)

		if len(mw.MonitorIDs) > 0 {
			found := false
			for _, id := range mw.MonitorIDs {
				if id == monitorID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if isInWindow(&mw, at) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func isInWindow(mw *MaintenanceWindow, at time.Time) bool {
	if mw.Recurring == "" {
		return !at.Before(mw.StartTime) && at.Before(mw.EndTime)
	}

	duration := mw.EndTime.Sub(mw.StartTime)
	switch mw.Recurring {
	case "daily":
		// Check if current time-of-day falls within the window
		startSec := mw.StartTime.Hour()*3600 + mw.StartTime.Minute()*60 + mw.StartTime.Second()
		atSec := at.Hour()*3600 + at.Minute()*60 + at.Second()
		endSec := startSec + int(duration.Seconds())
		if endSec > 86400 {
			return atSec >= startSec || atSec < (endSec-86400)
		}
		return atSec >= startSec && atSec < endSec
	case "weekly":
		startDay := mw.StartTime.Weekday()
		atDay := at.Weekday()
		if startDay == atDay {
			startSec := mw.StartTime.Hour()*3600 + mw.StartTime.Minute()*60 + mw.StartTime.Second()
			atSec := at.Hour()*3600 + at.Minute()*60 + at.Second()
			return atSec >= startSec && atSec < startSec+int(duration.Seconds())
		}
	case "monthly":
		if mw.StartTime.Day() == at.Day() {
			startSec := mw.StartTime.Hour()*3600 + mw.StartTime.Minute()*60 + mw.StartTime.Second()
			atSec := at.Hour()*3600 + at.Minute()*60 + at.Second()
			return atSec >= startSec && atSec < startSec+int(duration.Seconds())
		}
	}
	return false
}
