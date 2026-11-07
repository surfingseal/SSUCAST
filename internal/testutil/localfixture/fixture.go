// Package localfixture keeps captured school data and paid AI responses local.
package localfixture
import("os";"testing")
func Require(t *testing.T,path string){t.Helper();_,e:=os.Stat(path);if os.IsNotExist(e){if os.Getenv("REQUIRE_LOCAL_FIXTURES")=="true"{t.Fatalf("required local fixture absent: %s",path)};t.Skipf("local-only captured fixture not distributed: %s",path)};if e!=nil{t.Fatal(e)}}
