package node

import (
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"

	"sublink/models"
)

func TestDecodeHTTPProxyURL(t *testing.T) {
	proxy, err := DecodeHTTPProxyURL("https://user:p%40ss@[2001:db8::1]:8443#edge")
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Name != "edge" || proxy.Server != "2001:db8::1" || proxy.Port != 8443 {
		t.Fatalf("unexpected address fields: %+v", proxy)
	}
	if proxy.Username != "user" || proxy.Password != "p@ss" || !proxy.TLS {
		t.Fatalf("unexpected authentication fields: %+v", proxy)
	}
}

func TestScheduleClashToNodeLinksImportsHTTPAndSkipsUnsupported(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubScheduler{}, &models.SubSchedulerTarget{}, &models.Subcription{}, &models.Node{}, &models.SubcriptionNode{}, &models.SubLogs{}); err != nil {
		t.Fatal(err)
	}
	previous := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = previous })

	target := models.Subcription{Name: "master"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	scheduler := models.SubScheduler{Name: "Feed", URL: "https://example.test/sub", CronExpr: "0 * * * *"}
	if err := scheduler.AddWithTargets([]int{target.ID}); err != nil {
		t.Fatal(err)
	}

	count, err := scheduleClashToNodeLinks([]Proxy{
		{Name: "http", Type: "http", Server: "2001:db8::1", Port: 8080, Username: "user", Password: "p@ss", Tls: true},
		{Name: "hy2", Type: "hysteria2", Server: "hy2.example", Port: 8443, Password: "https://github.com/path#token"},
		{Name: "wireguard", Type: "wireguard", Server: "wg.example", Port: 51820},
		{Name: "invalid-hy2", Type: "hysteria2", Server: "placeholder.invalid"},
	}, scheduler.ID, scheduler.Name)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 supported nodes", count)
	}

	var imported models.Node
	if err := db.Where("scheduler_id = ?", scheduler.ID).First(&imported).Error; err != nil {
		t.Fatal(err)
	}
	proxy, err := DecodeHTTPProxyURL(imported.Link)
	if err != nil {
		t.Fatalf("decode imported link %q: %v", imported.Link, err)
	}
	if proxy.Name != "Feed_http" || proxy.Server != "2001:db8::1" || proxy.Username != "user" || proxy.Password != "p@ss" || !proxy.TLS {
		t.Fatalf("unexpected imported HTTP proxy: %+v", proxy)
	}

	var linkCount int64
	if err := db.Model(&models.SubcriptionNode{}).Where("subcription_id = ? AND node_id = ?", target.ID, imported.ID).Count(&linkCount).Error; err != nil {
		t.Fatal(err)
	}
	if linkCount != 1 {
		t.Fatalf("target subscription links = %d, want 1", linkCount)
	}

	var importedHY2 models.Node
	if err := db.Where("scheduler_id = ? AND name = ?", scheduler.ID, "Feed_hy2").First(&importedHY2).Error; err != nil {
		t.Fatal(err)
	}
	hy2, err := DecodeHY2URL(importedHY2.Link)
	if err != nil {
		t.Fatalf("decode imported Hysteria2 link: %v", err)
	}
	if hy2.Host != "hy2.example" || hy2.Port != 8443 || hy2.Password != "https://github.com/path#token" {
		t.Fatalf("unexpected imported Hysteria2 proxy: %+v", hy2)
	}
}

func TestLoadClashConfigFromURLRequestsClashContent(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubScheduler{}, &models.SubSchedulerTarget{}, &models.Subcription{}, &models.Node{}, &models.SubcriptionNode{}, &models.SubLogs{}); err != nil {
		t.Fatal(err)
	}
	previous := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = previous })

	target := models.Subcription{Name: "master"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	scheduler := models.SubScheduler{Name: "Feed", URL: "https://example.test/sub", CronExpr: "0 * * * *"}
	if err := scheduler.AddWithTargets([]int{target.ID}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "clash.meta" {
			t.Errorf("User-Agent = %q, want clash.meta", got)
		}
		if got := r.Header.Get("Accept"); got != "text/yaml, text/plain;q=0.9, */*;q=0.1" {
			t.Errorf("Accept = %q", got)
		}
		_, _ = w.Write([]byte("proxies:\n  - name: edge\n    type: ss\n    server: 192.0.2.1\n    port: 443\n    cipher: aes-128-gcm\n    password: secret\n"))
	}))
	defer server.Close()

	count, err := LoadClashConfigFromURL(scheduler.ID, server.URL, scheduler.Name)
	if err != nil {
		t.Fatalf("load subscription: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}
