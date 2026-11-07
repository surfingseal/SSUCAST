// Read-only Atlas connection check; never creates indexes or collections.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/config"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	if e := run(); e != nil {
		os.Exit(1)
	}
}
func run() (err error) {
	result := map[string]any{"database_changes": 0, "school_http_requests": 0, "vertex_requests": 0}
	defer func() {
		if err != nil {
			var command mongo.CommandError
			var dns *net.DNSError
			switch {
			case errors.As(err, &command):
				result["error_code"] = command.Code
				result["status"] = "mongo_command_error"
			case errors.As(err, &dns):
				result["status"] = "dns_error"
			case strings.Contains(err.Error(), "unable to authenticate") || strings.Contains(err.Error(), "AuthenticationFailed"):
				result["status"] = "authentication_failed"
			default:
				result["status"] = "connection_failed"
			}
		}
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(b))
	}()
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	result["database"] = cfg.Database
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, e := mongo.Connect(options.Client().ApplyURI(cfg.URI).SetServerSelectionTimeout(15 * time.Second).SetConnectTimeout(10 * time.Second))
	if e != nil {
		return e
	}
	defer client.Disconnect(context.Background())
	if e = client.Ping(ctx, nil); e != nil {
		return e
	}
	counts := map[string]int64{}
	for _, name := range []string{"crawl_sources", "crawled_contents", "content_classifications"} {
		n, e := client.Database(cfg.Database).Collection(name).CountDocuments(ctx, bson.M{})
		if e != nil {
			return e
		}
		counts[name] = n
	}
	result["status"] = "connected"
	result["collection_counts"] = counts
	return nil
}
