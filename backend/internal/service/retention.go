package service

import (
	"context"
	"log"
	"time"

	"server-monitor/backend/internal/db"
)

type RetentionWorker struct {
	DB *db.Database
}

func NewRetentionWorker(database *db.Database) *RetentionWorker {
	return &RetentionWorker{DB: database}
}

func (w *RetentionWorker) Start() {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			w.Cleanup()
		}
	}()
}

func (w *RetentionWorker) Cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Delete metrics older than 30 days
	cutoff := time.Now().AddDate(0, 0, -30)
	
	// Note: Since we use partitions, we could drop old partitions, but for simplicity we'll just delete rows for now.
	// In a real production system with partitions, we would drop tables like 'metrics_2023_01' etc.
	// Here we just run a DELETE query which might be slow but works for this scale.
	
	_, err := w.DB.GetPool().Exec(ctx, "DELETE FROM metrics WHERE timestamp < $1", cutoff)
	if err != nil {
		log.Printf("Retention cleanup failed: %v", err)
		return
	}
	
	log.Printf("Retention cleanup completed. Deleted metrics older than %s", cutoff.Format(time.RFC3339))
}
