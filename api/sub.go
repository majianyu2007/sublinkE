package api

import (
	"errors"
	"strconv"
	"strings"
	"sublink/dto"
	"sublink/models"
	"time"

	"github.com/gin-gonic/gin"
)

func SubTotal(c *gin.Context) {
	var Sub models.Subcription
	subs, err := Sub.List()
	count := len(subs)
	if err != nil {
		c.JSON(500, gin.H{
			"msg": "取得订阅总数失败",
		})
		return
	}
	c.JSON(200, gin.H{
		"code": "00000",
		"data": count,
		"msg":  "取得订阅总数",
	})
}

// 获取订阅列表
func SubGet(c *gin.Context) {
	var Sub models.Subcription
	Subs, err := Sub.List()
	if err != nil {
		c.JSON(500, gin.H{
			"msg": "node list error",
		})
		return
	}
	c.JSON(200, gin.H{
		"code": "00000",
		"data": Subs,
		"msg":  "node get",
	})
}

func parseIDList(value string) ([]int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, errors.New("invalid ID list")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// 添加节点
func SubAdd(c *gin.Context) {
	var sub models.Subcription
	name := c.PostForm("name")
	config := c.PostForm("config")
	nodes := c.PostForm("nodes")
	schedulerIDs, err := parseIDList(c.PostForm("scheduler_ids"))
	if err != nil {
		c.JSON(400, gin.H{"msg": "镜像订阅参数错误"})
		return
	}
	if name == "" || (nodes == "" && len(schedulerIDs) == 0) {
		c.JSON(400, gin.H{
			"msg": "订阅名称不能为空，且节点或镜像订阅至少选择一项",
		})
		return
	}
	sub.Nodes = []models.Node{}
	if nodes != "" {
		for _, v := range strings.Split(nodes, ",") {
			var node models.Node
			node.Name = v
			if err := node.Find(); err != nil || node.SchedulerID != nil {
				continue
			}
			sub.Nodes = append(sub.Nodes, node)
		}
	}
	if len(sub.Nodes) == 0 && len(schedulerIDs) == 0 {
		c.JSON(400, gin.H{"msg": "没有可用的手动节点或镜像订阅"})
		return
	}

	sub.Config = config
	sub.Name = name
	sub.CreateDate = time.Now().Format("2006-01-02 15:04:05")

	err = sub.SaveComposition(true, schedulerIDs)
	if err != nil {
		c.JSON(400, gin.H{
			"msg": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"code": "00000",
		"msg":  "添加成功",
	})
}

// 更新节点
func SubUpdate(c *gin.Context) {
	var sub models.Subcription
	name := c.PostForm("name")
	oldname := c.PostForm("oldname")
	config := c.PostForm("config")
	nodes := c.PostForm("nodes")
	schedulerIDs, err := parseIDList(c.PostForm("scheduler_ids"))
	if err != nil {
		c.JSON(400, gin.H{"msg": "镜像订阅参数错误"})
		return
	}
	if name == "" || (nodes == "" && len(schedulerIDs) == 0) {
		c.JSON(400, gin.H{
			"msg": "订阅名称不能为空，且节点或镜像订阅至少选择一项",
		})
		return
	}
	// 查找旧节点
	sub.Name = oldname
	err = sub.Find()
	if err != nil {
		c.JSON(400, gin.H{
			"msg": err.Error(),
		})
		return
	}
	// 更新节点
	sub.Config = config
	sub.Name = name
	sub.CreateDate = time.Now().Format("2006-01-02 15:04:05")
	sub.Nodes = []models.Node{}
	if nodes != "" {
		for _, v := range strings.Split(nodes, ",") {
			var node models.Node
			node.Name = v
			if err := node.Find(); err != nil || node.SchedulerID != nil {
				continue
			}
			sub.Nodes = append(sub.Nodes, node)
		}
	}
	if len(sub.Nodes) == 0 && len(schedulerIDs) == 0 {
		c.JSON(400, gin.H{"msg": "没有可用的手动节点或镜像订阅"})
		return
	}

	err = sub.SaveComposition(false, schedulerIDs)
	if err != nil {
		c.JSON(400, gin.H{
			"msg": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"code": "00000",
		"msg":  "更新成功",
	})
}

// 删除节点
func SubDel(c *gin.Context) {
	var sub models.Subcription
	id := c.Query("id")
	if id == "" {
		c.JSON(400, gin.H{
			"msg": "id 不能为空",
		})
		return
	}
	x, _ := strconv.Atoi(id)
	sub.ID = x
	err := sub.Find()
	if err != nil {
		c.JSON(400, gin.H{
			"msg": "查找失败",
		})
		return
	}
	err = sub.Del()
	if err != nil {
		c.JSON(400, gin.H{
			"msg": "删除失败",
		})
		return
	}
	c.JSON(200, gin.H{
		"code": "00000",
		"msg":  "删除成功",
	})
}

func SubSort(c *gin.Context) {
	var subNodeSort dto.SubcriptionNodeSortUpdate
	err := c.BindJSON(&subNodeSort)
	if err != nil {
		c.JSON(400, gin.H{"msg": "参数错误: " + err.Error()})
		return
	}

	var sub models.Subcription
	sub.ID = subNodeSort.ID
	err = sub.Sort(subNodeSort)

	if err != nil {
		c.JSON(400, gin.H{
			"msg": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"code": "00000",
		"msg":  "更新排序成功",
	})
}
