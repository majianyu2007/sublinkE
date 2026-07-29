package api

import (
	"testing"

	"sublink/models"
)

func TestIsRemoteSubscriptionNode(t *testing.T) {
	schedulerID := 1
	tests := []struct {
		name string
		node models.Node
		want bool
	}{
		{name: "manual remote subscription", node: models.Node{Link: "https://example.test/sub"}, want: true},
		{name: "scheduled HTTP proxy", node: models.Node{Link: "https://proxy.example:8443#edge", SchedulerID: &schedulerID}, want: false},
		{name: "scheduled HTTPS proxy with whitespace", node: models.Node{Link: " HTTPS://proxy.example:8443#edge", SchedulerID: &schedulerID}, want: false},
		{name: "non-HTTP proxy", node: models.Node{Link: "vless://example.test"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isRemoteSubscriptionNode(test.node); got != test.want {
				t.Fatalf("isRemoteSubscriptionNode() = %v, want %v", got, test.want)
			}
		})
	}
}
