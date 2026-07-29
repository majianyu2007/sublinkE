package models

import (
	"time"

	"gorm.io/gorm"
)

type SubScheduler struct {
	ID                   int `gorm:"primaryKey;autoIncrement"`
	Name                 string
	URL                  string
	CronExpr             string
	Enabled              bool
	SuccessCount         int        `gorm:"column:success_count"`
	LastRunTime          *time.Time `gorm:"type:datetime"`
	NextRunTime          *time.Time `gorm:"type:datetime"`
	CreatedAt            time.Time  `gorm:"autoCreateTime"`
	UpdatedAt            time.Time  `gorm:"autoUpdateTime"`
	TargetSubcriptionIDs []int      `gorm:"-" json:"TargetSubcriptionIDs"`
}

func (ss *SubScheduler) AddWithTargets(subscriptionIDs []int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ss).Error; err != nil {
			return err
		}
		return replaceSchedulerTargets(tx, ss.ID, subscriptionIDs)
	})
}

func (ss *SubScheduler) UpdateWithTargets(subscriptionIDs []int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(ss).Select("Name", "URL", "CronExpr", "Enabled").Updates(ss).Error; err != nil {
			return err
		}
		return replaceSchedulerTargets(tx, ss.ID, subscriptionIDs)
	})
}

// 查找节点是否重复
func (ss *SubScheduler) Find() error {
	return DB.Where("url = ? or name = ?", ss.URL, ss.Name).First(ss).Error
}

// List 获取所有订阅调度
func (ss *SubScheduler) List() ([]SubScheduler, error) {
	var schedulers []SubScheduler
	if err := DB.Find(&schedulers).Error; err != nil {
		return nil, err
	}
	var targets []SubSchedulerTarget
	if err := DB.Order("sort ASC, subcription_id ASC").Find(&targets).Error; err != nil {
		return nil, err
	}
	idsByScheduler := make(map[int][]int, len(schedulers))
	for _, target := range targets {
		idsByScheduler[target.SchedulerID] = append(idsByScheduler[target.SchedulerID], target.SubcriptionID)
	}
	for index := range schedulers {
		schedulers[index].TargetSubcriptionIDs = idsByScheduler[schedulers[index].ID]
	}
	return schedulers, nil
}

// ListEnabled 获取所有启用的订阅调度
func ListEnabled() ([]SubScheduler, error) {
	var schedulers []SubScheduler
	err := DB.Where("enabled = 1").Find(&schedulers).Error
	if err != nil {
		return nil, err
	}
	return schedulers, nil
}

func (ss *SubScheduler) Del() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var nodes []Node
		if err := tx.Where("scheduler_id = ?", ss.ID).Find(&nodes).Error; err != nil {
			return err
		}
		nodeIDs := make([]int, 0, len(nodes))
		for _, node := range nodes {
			nodeIDs = append(nodeIDs, node.ID)
		}
		if len(nodeIDs) > 0 {
			if err := tx.Where("node_id IN ?", nodeIDs).Delete(&SubcriptionNode{}).Error; err != nil {
				return err
			}
			if err := tx.Where("scheduler_id = ?", ss.ID).Delete(&Node{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("scheduler_id = ?", ss.ID).Delete(&SubSchedulerTarget{}).Error; err != nil {
			return err
		}
		return tx.Delete(ss).Error
	})
}

// UpdateRunTime 更新运行时间
func (ss *SubScheduler) UpdateRunTime(lastRun, nextRun *time.Time) error {
	return DB.Model(ss).Select("LastRunTime", "NextRunTime").Updates(map[string]interface{}{
		"LastRunTime": lastRun,
		"NextRunTime": nextRun,
	}).Error
}

// GetByID 根据ID获取订阅调度
func (ss *SubScheduler) GetByID(id int) error {
	return DB.Where("id = ?", id).First(ss).Error
}
