package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDotEnvPreservesProcessAndLiteralValues(t *testing.T) {
	t.Setenv("DOTENV_EXISTING", "process")
	t.Setenv("DOTENV_NEW", "")
	os.Unsetenv("DOTENV_NEW")
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte("DOTENV_EXISTING=file\nexport DOTENV_NEW='literal $(never-execute)'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotEnv(p); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DOTENV_EXISTING") != "process" || os.Getenv("DOTENV_NEW") != "literal $(never-execute)" {
		t.Fatal("precedence or literal parsing failed")
	}
}
