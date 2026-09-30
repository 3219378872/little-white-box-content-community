//go:build integration

package memory

import (
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/testutil"
	"testing"
)

func TestSQLFullMemoryIdentityContract(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	t.Cleanup(env.Close)
	for _, target := range []string{TargetMemory, TargetUser} {
		t.Run(target, func(t *testing.T) {
			env.TruncateAll(t, "memory_change", "core_memory_entry", "memory_target_lock")
			st := NewSQLStore(sqlx.NewSqlConnFromDB(env.DB), nil)
			memoryAddIdentityContract(t, st, target)
			memoryMutationContract(t, st, target)
		})
	}
}
