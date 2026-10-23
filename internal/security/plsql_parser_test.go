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
	} {
		if got := p.splitBySemicolons(script); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q, want %q", script, got, want)
		}
	}
}
