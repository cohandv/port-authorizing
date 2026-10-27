package security

import (
	"reflect"
	"testing"
)

func TestSplitBySemicolons(t *testing.T) {
	p := NewPLSQLParser()
	for script, want := range map[string][]string{
		"select 'a; b' || x from t;":                 {"select 'a; b' || x from t"},
		"select 'it''s; -- no' from t; select 2":     {"select 'it''s; -- no' from t", "select 2"},
		`select E'\'; x' , "a;b" from t`:             {`select E'\'; x' , "a;b" from t`},
		`select 'C:\'; select 1`:                     {`select 'C:\'`, "select 1"},
		"-- c; c\nselect 1 /* ; */ ; select 2":       {"select 1", "select 2"},
		"do $f$ begin perform 1; end $f$; select $1": {"do $f$ begin perform 1; end $f$", "select $1"},
		"BEGIN; UPDATE t SET a = 1; COMMIT;":         {"BEGIN", "UPDATE t SET a = 1", "COMMIT"},
		"select 1 as a$b$; select 2":                 {"select 1 as a$b$", "select 2"},
		// block comments nest: everything up to the matching */ is comment, so this is one statement
		"select 1 /* a /* b */ ; c */ ; select 2": {"select 1", "select 2"},
	} {
		if got := p.splitBySemicolons(script, false); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q, want %q", script, got, want)
		}
	}
}

// Both inputs were verified against PostgreSQL 16 to run DROP TABLE while lexing as a single SELECT under the
// old splitter (the second after SELECT set_config('standard_conforming_strings', 'off', false)).
func TestParseScriptExposesHiddenStatements(t *testing.T) {
	p := NewPLSQLParser()
	for _, script := range []string{
		"SELECT 1 /* a /* b */ ' */ ; DROP TABLE t; --'",
		`SELECT 'a\' , ' ; DROP TABLE t; --'`,
	} {
		found := false
		for _, sq := range p.ParseScript(script) {
			found = found || sq.Query == "DROP TABLE t"
		}
		if !found {
			t.Errorf("%q: DROP TABLE t not among parsed statements %+v", script, p.ParseScript(script))
		}
	}
}
