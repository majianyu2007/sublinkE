package node

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDecodeClashLeavesDynamicGroupsForMihomo(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "clash.yaml")
	const template = `proxies: []
proxy-groups:
  - name: "Hong Kong"
    type: url-test
    include-all-proxies: true
    filter: "(?i)HK"
    proxies: []
  - name: "Select"
    type: select
    proxies:
      - DIRECT
`
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := DecodeClash([]Proxy{
		{Name: "Feed_HK-1", Type: "http", Server: "192.0.2.1", Port: 443, Tls: true},
		{Name: "Feed_US-1", Type: "http", Server: "192.0.2.2", Port: 443, Tls: true},
	}, templatePath)
	if err != nil {
		t.Fatal(err)
	}

	var config struct {
		Proxies     []Proxy                  `yaml:"proxies"`
		ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(output, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Proxies) != 2 {
		t.Fatalf("generated proxies = %d, want 2", len(config.Proxies))
	}

	regional := config.ProxyGroups[0]["proxies"].([]interface{})
	if len(regional) != 0 {
		t.Fatalf("dynamic regional group received explicit proxies: %v", regional)
	}
	regular := config.ProxyGroups[1]["proxies"].([]interface{})
	if len(regular) != 3 {
		t.Fatalf("regular group proxies = %v, want DIRECT plus both generated proxies", regular)
	}
}

func TestDefaultClashTemplateRegionalFilters(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "template", "clash.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	samples := map[string]string{
		"🇭🇰 香港节点":  "Purens_HK-Mong Kok-1",
		"🚩 台湾节点":   "Purens_TW-Taipei-1",
		"🇯🇵 日本节点":  "Purens_JP-Tokyo-1",
		"🇸🇬 新加坡节点": "Purens_SG-Singapore-1",
		"🇺🇸 美国节点":  "Purens_US-Los Angeles-1",
		"🇰🇷 韩国节点":  "Purens_KR-Seoul-1",
		"🇪🇺 欧洲节点":  "Purens_RO-Bucharest-1",
	}
	found := make(map[string]bool, len(samples))
	for _, group := range config.ProxyGroups {
		name, _ := group["name"].(string)
		sample, expected := samples[name]
		if !expected {
			continue
		}
		filter, _ := group["filter"].(string)
		matcher, err := regexp.Compile(filter)
		if err != nil {
			t.Fatalf("%s has invalid filter %q: %v", name, filter, err)
		}
		if !matcher.MatchString(sample) {
			t.Errorf("%s filter %q does not match %q", name, filter, sample)
		}
		found[name] = true
	}
	for name := range samples {
		if !found[name] {
			t.Errorf("missing regional group %s", name)
		}
	}
}

func TestDefaultClashTemplatePreservesCustomPolicies(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "template", "clash.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ProxyGroups   []map[string]interface{} `yaml:"proxy-groups"`
		RuleProviders map[string]interface{}   `yaml:"rule-providers"`
		Rules         []string                 `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	groups := make(map[string]map[string]interface{}, len(config.ProxyGroups))
	for _, group := range config.ProxyGroups {
		name, _ := group["name"].(string)
		groups[name] = group
	}
	for _, name := range []string{
		"⭐ 自建优质节点", "⚡ 自建自动优选", "PROXY", "AI", "YouTube",
		"Netflix", "Disney+", "Streaming", "TikTok", "Telegram", "Social",
		"Google", "GitHub", "Microsoft", "Apple", "Vodafone-UK-WiFi",
		"Games", "Download", "CN", "AdBlock", "FINAL",
	} {
		if groups[name] == nil {
			t.Errorf("missing custom policy group %q", name)
		}
	}
	customFilter, _ := groups["⭐ 自建优质节点"]["exclude-filter"].(string)
	matcher, err := regexp.Compile(customFilter)
	if err != nil {
		t.Fatalf("invalid self-hosted exclusion filter %q: %v", customFilter, err)
	}
	if matcher.MatchString("HostDzire-Reality") || !matcher.MatchString("Purens_US-Los Angeles-1") {
		t.Errorf("self-hosted exclusion filter does not separate mirror nodes: %q", customFilter)
	}
	if len(config.RuleProviders) < 20 {
		t.Errorf("custom rule providers = %d, want at least 20", len(config.RuleProviders))
	}
	foundVodafoneRule := false
	for _, rule := range config.Rules {
		if rule == "DOMAIN,epdg.epc.mnc015.mcc234.pub.3gppnetwork.org,Vodafone-UK-WiFi" {
			foundVodafoneRule = true
			break
		}
	}
	if !foundVodafoneRule {
		t.Error("missing custom Vodafone Wi-Fi calling rule")
	}
}
