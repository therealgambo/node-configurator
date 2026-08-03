package facts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// DefaultCachePath is where Gather persists resolved Facts across boots, so
// the EC2 API isn't re-queried (and a transient IAM/network hiccup at early
// boot doesn't matter) once an instance's facts are known.
const DefaultCachePath = "/var/lib/node-configurator/facts.json"

type cacheEnvelope struct {
	Facts    Facts     `json:"facts"`
	CachedAt time.Time `json:"cachedAt"`
}

// loadCache returns cached Facts for wantInstanceID if present and, when
// ttl > 0, not older than ttl. An instance's type/vCPUs/memory/network
// don't change during its lifetime (barring a stop/modify/start cycle),
// so ttl <= 0 ("never expires") is the sane default; --refresh-facts lets
// an operator force a re-query after such a resize.
func loadCache(env *sysutil.Env, path, wantInstanceID string, ttl time.Duration) (Facts, bool) {
	data, err := os.ReadFile(env.Path(path))
	if err != nil {
		return Facts{}, false
	}
	var envlp cacheEnvelope
	if err := json.Unmarshal(data, &envlp); err != nil {
		return Facts{}, false
	}
	if wantInstanceID == "" || envlp.Facts.InstanceID != wantInstanceID {
		return Facts{}, false
	}
	if ttl > 0 && time.Since(envlp.CachedAt) > ttl {
		return Facts{}, false
	}
	return envlp.Facts, true
}

func saveCache(env *sysutil.Env, path string, f Facts) error {
	envlp := cacheEnvelope{Facts: f, CachedAt: time.Now()}
	data, err := json.MarshalIndent(envlp, "", "  ")
	if err != nil {
		return err
	}

	full := env.Path(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}
