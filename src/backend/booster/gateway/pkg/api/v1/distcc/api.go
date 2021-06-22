package distcc

import (
	"build-booster/gateway/pkg/api"
	"build-booster/server/pkg/engine/distcc"
)

const (
	queryGccVersionKey    = "gcc_version"
	queryUserKey          = "user"
	queryCompilerCountKey = "compiler_count"
	queryRequestCPUKey    = "request_cpu"
	querySuggestCPUKey    = "suggest_cpu"
	queryLeastCPUKey      = "least_cpu"
	queryCCacheEnabledKey = "ccache_enabled"
	queryBanDistcc        = "ban_distcc"
	queryBanAllBooster    = "ban_all_booster"
)

var (
	defaultMySQL distcc.MySQL

	// implements the keys that can be filtered with "In" during task list
	listTaskInKey = api.WithBasicGroup(api.GroupListTaskInKey, map[string]bool{
		queryGccVersionKey:    true,
		queryCompilerCountKey: true,
		queryUserKey:          true,
		queryCCacheEnabledKey: true,
		queryBanDistcc:        true,
		queryBanAllBooster:    true,
	})

	// implements the keys that can be filtered with "Gt" during task list
	listTaskGtKey = api.WithBasicGroup(api.GroupListTaskGtKey, map[string]bool{})

	// implements the keys that can be filtered with "Lt" during task list
	listTaskLtKey = api.WithBasicGroup(api.GroupListTaskLtKey, map[string]bool{})

	// implements the keys that can be filtered with "In" during project list
	listProjectInKey = api.WithBasicGroup(api.GroupListProjectInKey, map[string]bool{
		queryRequestCPUKey:    true,
		querySuggestCPUKey:    true,
		queryLeastCPUKey:      true,
		queryGccVersionKey:    true,
		queryCCacheEnabledKey: true,
		queryBanDistcc:        true,
		queryBanAllBooster:    true,
	})

	// implements the keys that can be filtered with "In" during whitelist list
	listWhitelistInKey = api.WithBasicGroup(api.GroupListWhitelistInKey, map[string]bool{})

	// implements the int keys
	intKey = api.WithBasicGroup(api.GroupIntKey, map[string]bool{
		queryCompilerCountKey: true,
	})

	// implements the int64 keys
	int64Key = api.WithBasicGroup(api.GroupInt64Key, map[string]bool{})

	// implements the bool keys
	boolKey = api.WithBasicGroup(api.GroupBoolKey, map[string]bool{
		queryCCacheEnabledKey: true,
		queryBanDistcc:        true,
		queryBanAllBooster:    true,
	})

	// implements the float64 keys
	float64Key = api.WithBasicGroup(api.GroupBoolKey, map[string]bool{
		queryRequestCPUKey: true,
		querySuggestCPUKey: true,
		queryLeastCPUKey:   true,
	})
)

// InitStorage After server init, the instances of manager, store ... etc. should be given into api handler.
func InitStorage() (err error) {
	defaultMySQL = api.GetDistCCServerAPIResource().MySQL
	return nil
}

func init() {
	api.RegisterInitFunc(InitStorage)
}
