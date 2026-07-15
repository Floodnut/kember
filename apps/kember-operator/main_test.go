package main

import "testing"

func TestValidateConcurrency(t *testing.T) {
	for _, test := range []struct {
		name      string
		workers   int
		execSlots int
		wantError bool
	}{
		{name: "reserved reconcile capacity", workers: 8, execSlots: 4},
		{name: "no exec slots", workers: 8, execSlots: 0, wantError: true},
		{name: "all workers can block", workers: 4, execSlots: 4, wantError: true},
		{name: "more slots than workers", workers: 4, execSlots: 5, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateConcurrency(test.workers, test.execSlots)
			if (err != nil) != test.wantError {
				t.Fatalf("validateConcurrency(%d, %d) error = %v, wantError %t", test.workers, test.execSlots, err, test.wantError)
			}
		})
	}
}
