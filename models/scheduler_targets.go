package models

import (
	"fmt"
	"sort"

	"gorm.io/gorm"
)

// SubSchedulerTarget assigns one mirrored scheduler group to an output subscription.
// Sort preserves the group order selected in the subscription editor.
type SubSchedulerTarget struct {
	SchedulerID   int `gorm:"primaryKey"`
	SubcriptionID int `gorm:"primaryKey"`
	Sort          int `gorm:"default:0"`
}

func migrateLegacySchedulerTargets(db *gorm.DB) error {
	if !db.Migrator().HasColumn("sub_schedulers", "target_subcription_id") {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT OR IGNORE INTO sub_scheduler_targets (scheduler_id, subcription_id, sort)
			SELECT id, target_subcription_id, 0
			FROM sub_schedulers
			WHERE target_subcription_id > 0
		`).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE sub_schedulers SET target_subcription_id = 0 WHERE target_subcription_id > 0").Error
	})
}

func normalizeIDs(ids []int) []int {
	result := make([]int, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func validateSubscriptionIDs(tx *gorm.DB, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&Subcription{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("one or more target subscriptions do not exist")
	}
	return nil
}

func validateSchedulerIDs(tx *gorm.DB, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&SubScheduler{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("one or more scheduler groups do not exist")
	}
	return nil
}

func replaceSchedulerTargets(tx *gorm.DB, schedulerID int, subscriptionIDs []int) error {
	subscriptionIDs = normalizeIDs(subscriptionIDs)
	if err := validateSubscriptionIDs(tx, subscriptionIDs); err != nil {
		return err
	}

	var oldTargets []SubSchedulerTarget
	if err := tx.Where("scheduler_id = ?", schedulerID).Find(&oldTargets).Error; err != nil {
		return err
	}
	oldSort := make(map[int]int, len(oldTargets))
	for _, target := range oldTargets {
		oldSort[target.SubcriptionID] = target.Sort
	}
	affected := make(map[int]struct{}, len(oldTargets)+len(subscriptionIDs))
	for _, target := range oldTargets {
		affected[target.SubcriptionID] = struct{}{}
	}
	for _, subscriptionID := range subscriptionIDs {
		affected[subscriptionID] = struct{}{}
	}

	if err := tx.Where("scheduler_id = ?", schedulerID).Delete(&SubSchedulerTarget{}).Error; err != nil {
		return err
	}
	for _, subscriptionID := range subscriptionIDs {
		sortValue, exists := oldSort[subscriptionID]
		if !exists {
			if err := tx.Model(&SubSchedulerTarget{}).
				Where("subcription_id = ?", subscriptionID).
				Select("COALESCE(MAX(sort), -1)").Scan(&sortValue).Error; err != nil {
				return err
			}
			sortValue++
		}
		target := SubSchedulerTarget{SchedulerID: schedulerID, SubcriptionID: subscriptionID, Sort: sortValue}
		if err := tx.Create(&target).Error; err != nil {
			return err
		}
	}

	targetIDs := make([]int, 0, len(affected))
	for subscriptionID := range affected {
		targetIDs = append(targetIDs, subscriptionID)
	}
	sort.Ints(targetIDs)
	for _, subscriptionID := range targetIDs {
		if err := rebuildTargetSubscription(tx, subscriptionID); err != nil {
			return err
		}
	}
	return nil
}

func replaceSubscriptionSchedulers(tx *gorm.DB, subscriptionID int, schedulerIDs []int) error {
	schedulerIDs = normalizeIDs(schedulerIDs)
	if err := validateSchedulerIDs(tx, schedulerIDs); err != nil {
		return err
	}
	if err := tx.Where("subcription_id = ?", subscriptionID).Delete(&SubSchedulerTarget{}).Error; err != nil {
		return err
	}
	for index, schedulerID := range schedulerIDs {
		target := SubSchedulerTarget{SchedulerID: schedulerID, SubcriptionID: subscriptionID, Sort: index}
		if err := tx.Create(&target).Error; err != nil {
			return err
		}
	}
	return rebuildTargetSubscription(tx, subscriptionID)
}

func rebuildTargetSubscription(tx *gorm.DB, subscriptionID int) error {
	if err := tx.Exec(`
		DELETE FROM subcription_nodes
		WHERE subcription_id = ?
		  AND node_id IN (SELECT id FROM nodes WHERE scheduler_id IS NOT NULL)
	`, subscriptionID).Error; err != nil {
		return fmt.Errorf("remove mirrored memberships: %w", err)
	}

	var nextSort int
	if err := tx.Model(&SubcriptionNode{}).
		Where("subcription_id = ?", subscriptionID).
		Select("COALESCE(MAX(sort), 0)").Scan(&nextSort).Error; err != nil {
		return fmt.Errorf("load subscription order: %w", err)
	}

	var targets []SubSchedulerTarget
	if err := tx.Where("subcription_id = ?", subscriptionID).
		Order("sort ASC, scheduler_id ASC").Find(&targets).Error; err != nil {
		return fmt.Errorf("load scheduler groups: %w", err)
	}
	for _, target := range targets {
		var nodes []Node
		if err := tx.Where("scheduler_id = ?", target.SchedulerID).Order("id ASC").Find(&nodes).Error; err != nil {
			return fmt.Errorf("load scheduler %d nodes: %w", target.SchedulerID, err)
		}
		for _, node := range nodes {
			nextSort++
			link := SubcriptionNode{SubcriptionID: subscriptionID, NodeID: node.ID, Sort: nextSort}
			if err := tx.Create(&link).Error; err != nil {
				return fmt.Errorf("add mirrored membership: %w", err)
			}
		}
	}
	return nil
}
