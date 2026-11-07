package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadDotEnv loads literal KEY=value settings without overriding process values.
// No interpolation, shell execution or command substitution is performed.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		s := strings.TrimSpace(scanner.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimPrefix(s, "export ")
		k, v, ok := strings.Cut(s, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if !ok || k == "" {
			return fmt.Errorf("dotenv line %d invalid", line)
		}
		for i, r := range k {
			if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
				return fmt.Errorf("dotenv key on line %d invalid", line)
			}
		}
		if strings.HasPrefix(v, "\"") {
			q, e := strconv.Unquote(v)
			if e != nil {
				return fmt.Errorf("dotenv quoted value on line %d invalid", line)
			}
			v = q
		} else if strings.HasPrefix(v, "'") {
			if len(v) < 2 || !strings.HasSuffix(v, "'") {
				return fmt.Errorf("dotenv quote on line %d invalid", line)
			}
			v = v[1 : len(v)-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if _, exists := os.LookupEnv(k); !exists {
			if err := os.Setenv(k, v); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
