package facts

import (
	"testing"
	"time"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

func TestCacheRoundTrip(t *testing.T) {
	env := &sysutil.Env{Root: t.TempDir()}
	path := "var/lib/node-configurator/facts.json"

	f := Facts{InstanceID: "i-0123456789abcdef0", InstanceType: "m6i.xlarge", VCPUs: 4, MemMiB: 16384, Source: "ec2-api"}
	if err := saveCache(env, path, f); err != nil {
		t.Fatalf("saveCache: %v", err)
	}

	got, ok := loadCache(env, path, "i-0123456789abcdef0", 0)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.InstanceType != "m6i.xlarge" || got.VCPUs != 4 {
		t.Fatalf("unexpected cached facts: %+v", got)
	}
}

func TestCacheMissOnDifferentInstanceID(t *testing.T) {
	env := &sysutil.Env{Root: t.TempDir()}
	path := "var/lib/node-configurator/facts.json"

	f := Facts{InstanceID: "i-old", Source: "ec2-api"}
	if err := saveCache(env, path, f); err != nil {
		t.Fatal(err)
	}

	_, ok := loadCache(env, path, "i-new", 0)
	if ok {
		t.Fatal("expected cache miss when instance ID changed (e.g. after stop/start on new hardware)")
	}
}

func TestCacheExpiresWithTTL(t *testing.T) {
	env := &sysutil.Env{Root: t.TempDir()}
	path := "var/lib/node-configurator/facts.json"

	f := Facts{InstanceID: "i-abc", Source: "ec2-api"}
	if err := saveCache(env, path, f); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)
	if _, ok := loadCache(env, path, "i-abc", time.Nanosecond); ok {
		t.Fatal("expected cache to be expired under a 1ns TTL")
	}
}

func TestCacheMissWhenAbsent(t *testing.T) {
	env := &sysutil.Env{Root: t.TempDir()}
	if _, ok := loadCache(env, "var/lib/node-configurator/facts.json", "i-abc", 0); ok {
		t.Fatal("expected cache miss when no file exists")
	}
}
