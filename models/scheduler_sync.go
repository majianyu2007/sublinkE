package models

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SyncScheduledNodes replaces one scheduler's imported node set and, when configured,
// merges that set into a target subscription without touching manually managed nodes.
func SyncScheduledNodes(schedulerID int, schedulerName string, incoming []Node) error {
	if schedulerID <= 0 {
		return errors.New("scheduler ID must be positive")
	}
	if len(incoming) == 0 {
		return errors.New("refusing to replace scheduled nodes with an empty set")
	}

	names := make([]string, 0, len(incoming))
	seen := make(map[string]struct{}, len(incoming))
	for i := range incoming {
		if incoming[i].Name == "" || incoming[i].Link == "" {
			return fmt.Errorf("scheduled node %d has an empty name or link", i)
		}
		if _, exists := seen[incoming[i].Name]; exists {
			return fmt.Errorf("duplicate scheduled node name %q", incoming[i].Name)
		}
		seen[incoming[i].Name] = struct{}{}
		names = append(names, incoming[i].Name)
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var scheduler SubScheduler
		if err := tx.First(&scheduler, schedulerID).Error; err != nil {
			return fmt.Errorf("load scheduler: %w", err)
		}

		// Claim nodes created by older SublinkE versions. The escaped underscore
		// ensures the scheduler prefix is treated literally rather than as LIKE's wildcard.
		legacyPrefix := escapeLike(schedulerName+"_") + "%"
		if err := tx.Model(&Node{}).
			Where("scheduler_id IS NULL AND source = ? AND name LIKE ? ESCAPE '\\'", "sublinkE", legacyPrefix).
			Update("scheduler_id", schedulerID).Error; err != nil {
			return fmt.Errorf("claim legacy scheduled nodes: %w", err)
		}

		var conflicts int64
		if err := tx.Model(&Node{}).
			Where("name IN ? AND (scheduler_id IS NULL OR scheduler_id <> ?)", names, schedulerID).
			Count(&conflicts).Error; err != nil {
			return fmt.Errorf("check node ownership: %w", err)
		}
		if conflicts != 0 {
			return fmt.Errorf("%d incoming node names conflict with manual nodes or another scheduler", conflicts)
		}

		for i := range incoming {
			incoming[i].SchedulerID = intPointer(schedulerID)
			incoming[i].Source = "sublinkE"
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "name"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"link", "dialer_proxy_name", "create_date", "source", "scheduler_id",
				}),
			}).Create(&incoming[i]).Error; err != nil {
				return fmt.Errorf("upsert scheduled node %q: %w", incoming[i].Name, err)
			}
		}

		var owned []Node
		if err := tx.Where("scheduler_id = ?", schedulerID).Find(&owned).Error; err != nil {
			return fmt.Errorf("load scheduled nodes: %w", err)
		}

		staleIDs := make([]int, 0)
		for _, node := range owned {
			if _, current := seen[node.Name]; !current {
				staleIDs = append(staleIDs, node.ID)
			}
		}
		if len(staleIDs) > 0 {
			if err := tx.Where("node_id IN ?", staleIDs).Delete(&SubcriptionNode{}).Error; err != nil {
				return fmt.Errorf("remove stale subscription links: %w", err)
			}
			if err := tx.Where("id IN ? AND scheduler_id = ?", staleIDs, schedulerID).Delete(&Node{}).Error; err != nil {
				return fmt.Errorf("delete stale scheduled nodes: %w", err)
			}
		}

		var targets []SubSchedulerTarget
		if err := tx.Where("scheduler_id = ?", schedulerID).Find(&targets).Error; err != nil {
			return fmt.Errorf("load scheduler targets: %w", err)
		}
		for _, target := range targets {
			if err := rebuildTargetSubscription(tx, target.SubcriptionID); err != nil {
				return fmt.Errorf("rebuild target subscription %d: %w", target.SubcriptionID, err)
			}
		}

		if err := tx.Model(&SubScheduler{}).Where("id = ?", schedulerID).
			Update("success_count", len(incoming)).Error; err != nil {
			return fmt.Errorf("update scheduler result: %w", err)
		}
		return nil
	})
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func intPointer(value int) *int {
	return &value
}
