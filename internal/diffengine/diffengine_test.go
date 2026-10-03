package diffengine

import (
	"reflect"
	"testing"
)

func TestParseUnifiedDiff(t *testing.T) {
	tests := []struct {
		name    string
		diff    string
		want    []ChangedFile
		wantErr bool
	}{
		{
			name: "single file with multiple hunks",
			diff: `diff --git a/pkg/math/calc.go b/pkg/math/calc.go
index 1234567..89abcdef 100644
--- a/pkg/math/calc.go
+++ b/pkg/math/calc.go
@@ -10,3 +10,5 @@ func Add(a, b int) int {
@@ -35 +37,2 @@ func Multiply(a, b int) int {
`,
			want: []ChangedFile{
				{
					Path: "pkg/math/calc.go",
					Lines: []LineRange{
						{Start: 10, End: 14},
						{Start: 37, End: 38},
					},
				},
			},
		},
		{
			name: "ignore non-go files and handle deletion hunk",
			diff: `diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1,5 +1,3 @@
diff --git a/service/user.go b/service/user.go
--- a/service/user.go
+++ b/service/user.go
@@ -42,2 +42,0 @@ func DeleteUser(id string) error {
`,
			want: []ChangedFile{
				{
					Path: "service/user.go",
					Lines: []LineRange{
						{Start: 42, End: 42},
					},
				},
			},
		},
		{
			name: "deleted file ignored",
			diff: `diff --git a/legacy.go b/dev/null
--- a/legacy.go
+++ /dev/null
@@ -1,10 +0,0 @@
`,
			want: nil,
		},
		{
			name: "brand new Go file with no prior version",
			diff: `diff --git a/newpkg/feature.go b/newpkg/feature.go
new file mode 100644
index 0000000..abcdef1
--- /dev/null
+++ b/newpkg/feature.go
@@ -0,0 +1,25 @@
+package newpkg
+
+func NewFeature() string {
+	return "ok"
+}
`,
			want: []ChangedFile{
				{
					Path: "newpkg/feature.go",
					Lines: []LineRange{
						{Start: 1, End: 25},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUnifiedDiff([]byte(tt.diff))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseUnifiedDiff() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseUnifiedDiff() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
