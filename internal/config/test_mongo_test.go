package config

import "testing"

func TestIsolatedScheduleMongoGuard(t *testing.T) {
	name := "ssu_schedule_apply_test_0123456789abcdef01234567"
	if e := ValidateIsolatedTestMongo("mongodb://127.0.0.1:27030", name); e != nil {
		t.Fatal(e)
	}
	for i, uri := range []string{"mongodb+srv://example.mongodb.net/", "mongodb://example.mongodb.net:27017", "mongodb://user:password@127.0.0.1:27030", "mongodb://127.0.0.1:27030/ssu_crawler", "mongodb://127.0.0.1:27030/?replicaSet=remote"} {
		if ValidateIsolatedTestMongo(uri, name) == nil {
			t.Errorf("unsafe test target %d accepted", i)
		}
	}
	for _, db := range []string{"ssu_crawler", "ssu_recent_20261106", "admin", "ssu_schedule_apply_test_existing"} {
		if ValidateIsolatedTestMongo("mongodb://127.0.0.1:27030", db) == nil {
			t.Fatal("unsafe database accepted")
		}
	}
}
