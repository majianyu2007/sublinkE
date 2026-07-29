package models

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&SubScheduler{}, &SubSchedulerTarget{}, &Subcription{}, &Node{}, &SubcriptionNode{}, &SubLogs{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	return db
}

func TestSyncScheduledNodesMirrorsAndMerges(t *testing.T) {
	db := openSyncTestDB(t)
	target := Subcription{Name: "master"}
	other := Subcription{Name: "other"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	scheduler := SubScheduler{Name: "Purens", URL: "https://example.test/sub", CronExpr: "0 * * * *", Enabled: true}
	if err := scheduler.AddWithTargets([]int{target.ID, other.ID}); err != nil {
		t.Fatal(err)
	}

	manual := Node{Name: "manual", Link: "ss://manual", Source: "manual"}
	legacyCurrent := Node{Name: "Purens_current", Link: "http://old.example:80#old", Source: "sublinkE"}
	legacyStale := Node{Name: "Purens_stale", Link: "", Source: "sublinkE"}
	for _, node := range []*Node{&manual, &legacyCurrent, &legacyStale} {
		if err := db.Create(node).Error; err != nil {
			t.Fatal(err)
		}
	}
	links := []SubcriptionNode{
		{SubcriptionID: target.ID, NodeID: manual.ID, Sort: 7},
		{SubcriptionID: target.ID, NodeID: legacyCurrent.ID, Sort: 8},
		{SubcriptionID: other.ID, NodeID: legacyCurrent.ID, Sort: 1},
		{SubcriptionID: target.ID, NodeID: legacyStale.ID, Sort: 9},
	}
	if err := db.Create(&links).Error; err != nil {
		t.Fatal(err)
	}

	incoming := []Node{
		{Name: "Purens_current", Link: "http://new.example:8080#current", CreateDate: "now"},
		{Name: "Purens_added", Link: "socks5://new.example:1080#added", CreateDate: "now"},
	}
	if err := SyncScheduledNodes(scheduler.ID, scheduler.Name, incoming); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var nodes []Node
	if err := db.Order("name").Find(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want manual plus two scheduled nodes", len(nodes))
	}
	byName := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		byName[node.Name] = node
	}
	if byName["manual"].SchedulerID != nil {
		t.Fatal("manual node was claimed by scheduler")
	}
	if _, exists := byName["Purens_stale"]; exists {
		t.Fatal("stale scheduled node was not deleted")
	}
	if got := byName["Purens_current"].Link; got != incoming[0].Link {
		t.Fatalf("updated link = %q, want %q", got, incoming[0].Link)
	}
	for _, name := range []string{"Purens_current", "Purens_added"} {
		owner := byName[name].SchedulerID
		if owner == nil || *owner != scheduler.ID {
			t.Fatalf("node %q owner = %v, want scheduler %d", name, owner, scheduler.ID)
		}
	}

	var targetLinks []SubcriptionNode
	if err := db.Where("subcription_id = ?", target.ID).Order("sort").Find(&targetLinks).Error; err != nil {
		t.Fatal(err)
	}
	if len(targetLinks) != 3 {
		t.Fatalf("target has %d links, want manual plus two scheduled nodes", len(targetLinks))
	}
	if targetLinks[0].NodeID != manual.ID || targetLinks[0].Sort != 7 {
		t.Fatalf("manual membership changed: %+v", targetLinks[0])
	}
	if targetLinks[1].NodeID != byName[incoming[0].Name].ID || targetLinks[1].Sort != 8 {
		t.Fatalf("first scheduled membership = %+v", targetLinks[1])
	}
	if targetLinks[2].NodeID != byName[incoming[1].Name].ID || targetLinks[2].Sort != 9 {
		t.Fatalf("second scheduled membership = %+v", targetLinks[2])
	}
	var otherCount int64
	if err := db.Model(&SubcriptionNode{}).Where("subcription_id = ?", other.ID).Count(&otherCount).Error; err != nil {
		t.Fatal(err)
	}
	if otherCount != int64(len(incoming)) {
		t.Fatalf("second target has %d mirrored nodes, want %d", otherCount, len(incoming))
	}

	var updatedScheduler SubScheduler
	if err := db.First(&updatedScheduler, scheduler.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedScheduler.SuccessCount != len(incoming) {
		t.Fatalf("success count = %d, want %d", updatedScheduler.SuccessCount, len(incoming))
	}
}

func TestSyncScheduledNodesRejectsEmptyAndManualConflicts(t *testing.T) {
	db := openSyncTestDB(t)
	scheduler := SubScheduler{Name: "feed", URL: "https://example.test/sub", CronExpr: "0 * * * *"}
	if err := db.Create(&scheduler).Error; err != nil {
		t.Fatal(err)
	}
	manual := Node{Name: "feed_same", Link: "ss://manual", Source: "manual"}
	if err := db.Create(&manual).Error; err != nil {
		t.Fatal(err)
	}

	if err := SyncScheduledNodes(scheduler.ID, scheduler.Name, nil); err == nil {
		t.Fatal("empty sync succeeded")
	}
	incoming := []Node{{Name: manual.Name, Link: "ss://automatic"}}
	if err := SyncScheduledNodes(scheduler.ID, scheduler.Name, incoming); err == nil {
		t.Fatal("sync overwrote a manual node")
	}

	var persisted Node
	if err := db.First(&persisted, manual.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Link != manual.Link || persisted.SchedulerID != nil {
		t.Fatalf("manual node changed after rejected sync: %+v", persisted)
	}
}

func TestSaveCompositionSelectsWholeSchedulerGroups(t *testing.T) {
	db := openSyncTestDB(t)
	first := SubScheduler{Name: "first", URL: "https://example.test/first", CronExpr: "0 * * * *"}
	second := SubScheduler{Name: "second", URL: "https://example.test/second", CronExpr: "0 * * * *"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	manual := Node{Name: "manual", Link: "ss://manual", Source: "manual"}
	firstOne := Node{Name: "first_one", Link: "ss://first-one", Source: "sublinkE", SchedulerID: intPointer(first.ID)}
	firstTwo := Node{Name: "first_two", Link: "ss://first-two", Source: "sublinkE", SchedulerID: intPointer(first.ID)}
	secondOne := Node{Name: "second_one", Link: "ss://second-one", Source: "sublinkE", SchedulerID: intPointer(second.ID)}
	for _, node := range []*Node{&manual, &firstOne, &firstTwo, &secondOne} {
		if err := db.Create(node).Error; err != nil {
			t.Fatal(err)
		}
	}

	output := Subcription{Name: "output", Config: "{}", Nodes: []Node{manual}}
	if err := output.SaveComposition(true, []int{first.ID, second.ID}); err != nil {
		t.Fatalf("create composition: %v", err)
	}
	var links []SubcriptionNode
	if err := db.Where("subcription_id = ?", output.ID).Order("sort ASC").Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	want := []int{manual.ID, firstOne.ID, firstTwo.ID, secondOne.ID}
	if len(links) != len(want) {
		t.Fatalf("composition has %d nodes, want %d", len(links), len(want))
	}
	for index, nodeID := range want {
		if links[index].NodeID != nodeID {
			t.Fatalf("composition node %d = %d, want %d", index, links[index].NodeID, nodeID)
		}
	}

	output.Nodes = []Node{manual}
	if err := output.SaveComposition(false, []int{second.ID}); err != nil {
		t.Fatalf("update composition: %v", err)
	}
	links = nil
	if err := db.Where("subcription_id = ?", output.ID).Order("sort ASC").Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].NodeID != manual.ID || links[1].NodeID != secondOne.ID {
		t.Fatalf("updated composition = %+v, want manual plus second group", links)
	}
}

func TestMigrateLegacySchedulerTargetOnce(t *testing.T) {
	db := openSyncTestDB(t)
	target := Subcription{Name: "target"}
	scheduler := SubScheduler{Name: "legacy", URL: "https://example.test/legacy", CronExpr: "0 * * * *"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&scheduler).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE sub_schedulers ADD COLUMN target_subcription_id INTEGER DEFAULT 0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE sub_schedulers SET target_subcription_id = ? WHERE id = ?", target.ID, scheduler.ID).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacySchedulerTargets(db); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := migrateLegacySchedulerTargets(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	var targets int64
	if err := db.Model(&SubSchedulerTarget{}).
		Where("scheduler_id = ? AND subcription_id = ?", scheduler.ID, target.ID).
		Count(&targets).Error; err != nil {
		t.Fatal(err)
	}
	if targets != 1 {
		t.Fatalf("migrated target rows = %d, want 1", targets)
	}
	var legacyTarget int
	if err := db.Raw("SELECT target_subcription_id FROM sub_schedulers WHERE id = ?", scheduler.ID).Scan(&legacyTarget).Error; err != nil {
		t.Fatal(err)
	}
	if legacyTarget != 0 {
		t.Fatalf("legacy target remains %d, want 0 after cutover", legacyTarget)
	}
}
