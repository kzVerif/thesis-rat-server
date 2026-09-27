package service

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

func TestEnsureRoomReadPermission(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, tc := range []struct {
		name       string
		required   bool
		failAt     int
		wantWrites int
	}{
		{"unrelated permissions", false, 0, 0},
		{"dependent permission", true, 0, 2},
		{"permission creation failure", true, 1, 1},
		{"assignment failure", true, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			db := sql.OpenDB(&logsTestDB{
				query: func(_ string, args []driver.NamedValue) (driver.Rows, error) {
					if len(args) != 2 || args[0].Value != "role-id" || args[1].Value != "agent.manage" {
						t.Fatalf("unexpected role: %v", args)
					}
					return &logsTestRows{[]string{"exists"}, [][]driver.Value{{tc.required}}}, nil
				},
				exec: func(_ string, args []driver.NamedValue) (driver.Result, error) {
					writes++
					if writes == 1 && args[0].Value != RoomsReadPermission {
						t.Fatalf("unexpected permission: %v", args)
					}
					if writes == 2 && (args[0].Value != "role-id" || args[1].Value != RoomsReadPermission) {
						t.Fatalf("unexpected assignment: %v", args)
					}
					if writes == tc.failAt {
						return nil, failure
					}
					return driver.RowsAffected(1), nil
				},
			})
			defer db.Close()
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			err = ensureRoomReadPermission(tx, "role-id")
			if tc.failAt > 0 {
				if !errors.Is(err, failure) {
					t.Fatalf("error = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if writes != tc.wantWrites {
				t.Fatalf("writes = %d, want %d", writes, tc.wantWrites)
			}
		})
	}
}
