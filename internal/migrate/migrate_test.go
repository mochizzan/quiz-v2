package migrate

import (
	"strings"
	"testing"

	"quiz-v2/migrations"
)

func TestStatements(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		want  []string
		chomp string // substring expected in the error, when in error
	}{
		{
			name: "statements plus script header comments",
			src: `-- 0001 — accounts schema.
-- Spec: somewhere.

CREATE TABLE IF NOT EXISTS users (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  PRIMARY KEY (id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS seats (
  user_id INT UNSIGNED NULL
);
`,
			want: []string{
				"CREATE TABLE IF NOT EXISTS users (\n  id INT UNSIGNED NOT NULL AUTO_INCREMENT,\n  PRIMARY KEY (id)\n) ENGINE=InnoDB",
				"CREATE TABLE IF NOT EXISTS seats (\n  user_id INT UNSIGNED NULL\n)",
			},
		},
		{
			name: "single statement without trailing newline",
			src:  "INSERT INTO majors (name) VALUES ('IPA');",
			want: []string{"INSERT INTO majors (name) VALUES ('IPA')"},
		},
		{
			name: "comment-only file changes nothing",
			src:  "-- nothing to do\n\n",
		},
		{
			name:  "unterminated statement is an error",
			src:   "CREATE TABLE t (id int)\n",
			chomp: "unterminated",
		},
		{
			name:  "windows line endings work",
			src:   "DELETE FROM x;\r\nINSERT INTO y VALUES (1);\r\n",
			chomp: "",
			want:  []string{"DELETE FROM x", "INSERT INTO y VALUES (1)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts, err := Statements(tt.src)
			if tt.chomp != "" {
				if err == nil || !strings.Contains(err.Error(), tt.chomp) {
					t.Fatalf("Statements() error = %v, want containing %q", err, tt.chomp)
				}
				return
			}
			if err != nil {
				t.Fatalf("Statements() error = %v", err)
			}
			if len(stmts) != len(tt.want) {
				t.Fatalf("Statements() = %d statements, want %d: %q", len(stmts), len(tt.want), stmts)
			}
			for i := range tt.want {
				if stmts[i] != tt.want[i] {
					t.Errorf("statement[%d] =\n%q\nwant\n%q", i, stmts[i], tt.want[i])
				}
			}
		})
	}
}

// The nine real files must all split cleanly today; a later file that grows
// a DELIMITER block or an embedded semicolon fails this test before it can
// half-apply.
func TestStatementsRealMigrationsSplit(t *testing.T) {
	names, err := Files()
	if err != nil {
		t.Fatalf("Files() error = %v", err)
	}
	if len(names) != 9 {
		t.Fatalf("Files() = %v, want the 9 schema migrations", names)
	}
	for _, name := range names {
		source, err := migrations.SQL.ReadFile(name + ".up.sql")
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		stmts, err := Statements(string(source))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(stmts) == 0 {
			t.Fatalf("%s: split produced no statements", name)
		}
		for i, stmt := range stmts {
			if strings.Contains(stmt, ";\n") || strings.HasSuffix(stmt, ";") {
				t.Errorf("%s statement[%d] still carries a semicolon: %q", name, i, stmt)
			}
		}
	}
}

func TestFilesSortedOldestFirst(t *testing.T) {
	names, err := Files()
	if err != nil {
		t.Fatalf("Files() error = %v", err)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("names not strictly increasing: %q then %q", names[i-1], names[i])
		}
	}
}
