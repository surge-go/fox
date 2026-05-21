package redis

import (
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func TestNewClientStandalone(t *testing.T) {
	client, err := NewClient(&Config{
		Addrs: []string{"127.0.0.1:6379"},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if _, ok := client.(*goredis.Client); !ok {
		t.Fatalf("NewClient() type = %T, want *redis.Client", client)
	}
}

func TestNewClientSentinel(t *testing.T) {
	client, err := NewClient(&Config{
		Mode:  ModeSentinel,
		Addrs: []string{"127.0.0.1:26379"},
		Sentinel: &SentinelConfig{
			MasterName: "mymaster",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if _, ok := client.(*goredis.Client); !ok {
		t.Fatalf("NewClient() type = %T, want *redis.Client", client)
	}
}

func TestNewClientCluster(t *testing.T) {
	client, err := NewClient(&Config{
		Mode:    ModeCluster,
		Addrs:   []string{"127.0.0.1:6379"},
		Cluster: &ClusterConfig{},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if _, ok := client.(*goredis.ClusterClient); !ok {
		t.Fatalf("NewClient() type = %T, want *redis.ClusterClient", client)
	}
}

func TestNewClientRejectsInvalidConfig(t *testing.T) {
	client, err := NewClient(&Config{})
	if err == nil {
		client.Close()
		t.Fatal("NewClient() error = nil, want error")
	}
}
