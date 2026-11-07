package config

import (
	"fmt"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"net/url"
	"os"
	"strings"
)

// URI is secret material: never include this structure in a diagnostic dump.
type MongoConnection struct {
	URI      string `json:"-"`
	Database string `json:"database"`
}

func (c MongoConnection) String() string { return "MongoDB(database=" + c.Database + ")" }

// Explicit CLI flags > process environment > .env > standard local defaults.
// Call only in an explicit execution path; Preview does not load secrets.
func LoadMongoConnection(uriOverride, databaseOverride string) (MongoConnection, error) {
	if e := LoadDotEnv(".env"); e != nil {
		return MongoConnection{}, e
	}
	c := MongoConnection{URI: uriOverride, Database: databaseOverride}
	if c.URI == "" {
		c.URI = os.Getenv("MONGODB_URI")
	}
	if c.URI == "" {
		c.URI = "mongodb://localhost:27017"
	}
	if c.Database == "" {
		c.Database = os.Getenv("MONGODB_DATABASE")
	}
	if c.Database == "" {
		c.Database = "crawlserver"
	}
	if options.Client().ApplyURI(c.URI).Validate() != nil {
		return MongoConnection{}, fmt.Errorf("invalid MONGODB_URI (redacted)")
	}
	if strings.TrimSpace(c.Database) == "" || strings.ContainsAny(c.Database, "/\\. \"$\x00") {
		return MongoConnection{}, fmt.Errorf("invalid MONGODB_DATABASE")
	}
	return c, nil
}
func RedactMongoError(err error, uri string) error {
	if err == nil {
		return nil
	}
	message := strings.ReplaceAll(err.Error(), uri, "[redacted MongoDB URI]")
	if u, e := url.Parse(uri); e == nil && u.User != nil {
		if password, ok := u.User.Password(); ok && password != "" {
			message = strings.ReplaceAll(message, password, "[redacted]")
			message = strings.ReplaceAll(message, url.QueryEscape(password), "[redacted]")
		}
	}
	return fmt.Errorf("%s", message)
}
