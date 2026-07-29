package models

import (
	"fmt"
	"sublink/dto"

	"gorm.io/gorm"
)

type Subcription struct {
	ID            int
	Name          string
	Config        string    `gorm:"embedded"`
	Nodes         []Node    `gorm:"many2many:subcription_nodes;" json:"-"` // 多对多关系
	SubLogs       []SubLogs `gorm:"foreignKey:SubcriptionID;"`             // 一对多关系 约束父表被删除子表记录跟着删除
	CreateDate    string
	NodesWithSort []NodeWithSort `gorm:"-" json:"Nodes"`
	SchedulerIDs  []int          `gorm:"-" json:"SchedulerIDs"`
}

type SubcriptionNode struct {
	SubcriptionID int `gorm:"primaryKey"`
	NodeID        int `gorm:"primaryKey"`
	Sort          int `gorm:"default:0"`
}

type NodeWithSort struct {
	Node
	Sort int `json:"Sort"`
}

// Add 添加订阅
func (sub *Subcription) Add() error {
	return DB.Create(sub).Error
}

// 添加节点列表建立多对多关系
func (sub *Subcription) AddNode() error {
	return DB.Model(sub).Association("Nodes").Append(sub.Nodes)
}

// 更新订阅
func (sub *Subcription) Update() error {
	return DB.Where("id = ? or name = ?", sub.ID, sub.Name).Updates(sub).Error
}

// 更新节点列表建立多对多关系
func (sub *Subcription) UpdateNodes() error {
	return DB.Model(sub).Association("Nodes").Replace(sub.Nodes)
}

// 查找订阅
func (sub *Subcription) Find() error {
	return DB.Where("id = ? or name = ?", sub.ID, sub.Name).First(sub).Error
}

// 读取订阅
func (sub *Subcription) GetSub() error {
	// err := DB.Find(sub).Error
	// if err != nil {
	// 	return err
	// }
	return DB.Table("nodes").
		Joins("left join subcription_nodes ON subcription_nodes.node_id = nodes.id").
		Where("subcription_nodes.subcription_id = ?", sub.ID).
		Order("subcription_nodes.sort ASC, nodes.id ASC").Find(&sub.Nodes).Error
}

// 订阅列表

func (sub *Subcription) List() ([]Subcription, error) {
	var subs []Subcription
	// 先查所有订阅
	err := DB.Find(&subs).Error
	if err != nil {
		return nil, err
	}

	for i := range subs {
		// 查询订阅对应的节点和sort字段，按sort和node.id排序
		var nodesWithSort []NodeWithSort
		err := DB.Table("nodes").
			Select("nodes.*, subcription_nodes.sort").
			Joins("LEFT JOIN subcription_nodes ON subcription_nodes.node_id = nodes.id").
			Where("subcription_nodes.subcription_id = ?", subs[i].ID).
			Order("subcription_nodes.sort ASC, nodes.id ASC").
			Scan(&nodesWithSort).Error
		if err != nil {
			return nil, err
		}
		subs[i].NodesWithSort = nodesWithSort
		var schedulerTargets []SubSchedulerTarget
		if err := DB.Where("subcription_id = ?", subs[i].ID).
			Order("sort ASC, scheduler_id ASC").Find(&schedulerTargets).Error; err != nil {
			return nil, err
		}
		subs[i].SchedulerIDs = make([]int, 0, len(schedulerTargets))
		for _, target := range schedulerTargets {
			subs[i].SchedulerIDs = append(subs[i].SchedulerIDs, target.SchedulerID)
		}

		// 查询日志
		err = DB.Model(&subs[i]).Association("SubLogs").Find(&subs[i].SubLogs)
		if err != nil {
			return nil, err
		}
	}

	return subs, nil
}

func (sub *Subcription) SaveComposition(create bool, schedulerIDs []int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if create {
			if err := tx.Omit("Nodes", "SubLogs").Create(sub).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&Subcription{}).Where("id = ?", sub.ID).Updates(map[string]interface{}{
				"name":        sub.Name,
				"config":      sub.Config,
				"create_date": sub.CreateDate,
			}).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("subcription_id = ?", sub.ID).Delete(&SubcriptionNode{}).Error; err != nil {
			return err
		}
		for index, node := range sub.Nodes {
			if node.SchedulerID != nil {
				return fmt.Errorf("scheduled nodes must be selected through scheduler groups")
			}
			link := SubcriptionNode{SubcriptionID: sub.ID, NodeID: node.ID, Sort: index + 1}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return replaceSubscriptionSchedulers(tx, sub.ID, schedulerIDs)
	})
}

func (sub *Subcription) IPlogUpdate() error {
	return DB.Model(sub).Association("SubLogs").Replace(&sub.SubLogs)
}

// 删除订阅
func (sub *Subcription) Del() error {
	err := DB.Model(sub).Association("Nodes").Clear()
	if err != nil {
		return err
	}
	if err := DB.Where("subcription_id = ?", sub.ID).Delete(&SubSchedulerTarget{}).Error; err != nil {
		return err
	}
	return DB.Delete(sub).Error
}

func (sub *Subcription) Sort(subNodeSort dto.SubcriptionNodeSortUpdate) error {
	tx := DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %w", tx.Error)
	}
	for _, item := range subNodeSort.NodeSort {
		err := tx.Model(&SubcriptionNode{}).
			Where("subcription_id = ? AND node_id = ?", subNodeSort.ID, item.ID).
			Update("sort", item.Sort).Error

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("更新节点排序失败: %w", err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}
