package config

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMongoConnectionEnvPriorityAndSecretRedaction(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("MONGODB_URI", "mongodb://localhost:27031")
	t.Setenv("MONGODB_DATABASE", "process_db")
	if e := os.WriteFile(".env", []byte("MONGODB_URI=mongodb://localhost:27032\nMONGODB_DATABASE=dotenv_db\n"), 0600); e != nil {
		t.Fatal(e)
	}
	c, e := LoadMongoConnection("", "")
	if e != nil || c.URI != "mongodb://localhost:27031" || c.Database != "process_db" {
		t.Fatal(c, e)
	}
	c, e = LoadMongoConnection("mongodb://localhost:27033", "flag_db")
	if e != nil || c.URI != "mongodb://localhost:27033" || c.Database != "flag_db" {
		t.Fatal(c, e)
	}
	uri := "mongodb://sample_user:test-secret@localhost:27017"
	err := RedactMongoError(errors.New("error "+uri+" password=test-secret"), uri)
	if strings.Contains(err.Error(), "test-secret") {
		t.Fatal("secret leaked")
	}
	raw, e := json.Marshal(MongoConnection{URI: uri, Database: "sample"})
	if e != nil || strings.Contains(string(raw), "secret") || strings.Contains((MongoConnection{URI: uri}).String(), "secret") {
		t.Fatal("diagnostic URI exposure")
	}
}
