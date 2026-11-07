package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var isolatedDB = regexp.MustCompile(`^(?:crawltest_|ssu_schedule_apply_test_)[a-f0-9]{24}$`)

// ValidateIsolatedTestMongo is called BEFORE opening/dropping/seeding any DB.
// Atlas/SRV, credentials, non-loopback hosts and URI-embedded DBs are rejected.
func ValidateIsolatedTestMongo(uri, database string) error {
	u, e := url.Parse(uri)
	if e != nil || u.Scheme != "mongodb" || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") || !isolatedDB.MatchString(database) {
		return fmt.Errorf("isolated test requires loopback mongodb URI and fresh test DB name")
	}
	for _, host := range strings.Split(u.Host, ",") {
		name, _, err := net.SplitHostPort(host)
		if err != nil {
			name = host
		}
		ip := net.ParseIP(strings.Trim(name, "[]"))
		if name != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("refusing non-loopback/Atlas test target")
		}
	}
	if u.Host == "" {
		return fmt.Errorf("empty test host")
	}
	return nil
}
