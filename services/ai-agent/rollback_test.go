package main

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rsWithRevision builds an in-memory ReplicaSet. An empty revision means the
// annotation is absent.
func rsWithRevision(name, revision string) appsv1.ReplicaSet {
	rs := appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if revision != "" {
		rs.Annotations = map[string]string{deploymentRevisionAnnotation: revision}
	}
	return rs
}

func TestSelectPreviousReplicaSet(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		replicaSets []appsv1.ReplicaSet
		wantName    string
		wantRev     int64
		wantErr     string
	}{
		{
			name:    "NumericOrder_RegressionD1",
			current: "11",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-9", "9"),
				rsWithRevision("rs-10", "10"),
				rsWithRevision("rs-11", "11"),
			},
			wantName: "rs-10",
			wantRev:  10,
		},
		{
			name:    "MultiDigitGap",
			current: "10",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-1", "1"),
				rsWithRevision("rs-2", "2"),
				rsWithRevision("rs-10", "10"),
			},
			wantName: "rs-2",
			wantRev:  2,
		},
		{
			name:    "UnsortedInput",
			current: "11",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-11", "11"),
				rsWithRevision("rs-9", "9"),
				rsWithRevision("rs-10", "10"),
			},
			wantName: "rs-10",
			wantRev:  10,
		},
		{
			name:    "IgnoresHigherThanCurrent_D2",
			current: "5",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-4", "4"),
				rsWithRevision("rs-6", "6"),
				rsWithRevision("rs-5", "5"),
			},
			wantName: "rs-4",
			wantRev:  4,
		},
		{
			name:    "SkipsMissingAnnotation",
			current: "3",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-none", ""),
				rsWithRevision("rs-2", "2"),
				rsWithRevision("rs-3", "3"),
			},
			wantName: "rs-2",
			wantRev:  2,
		},
		{
			name:    "SkipsNonNumeric",
			current: "3",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-abc", "abc"),
				rsWithRevision("rs-2", "2"),
				rsWithRevision("rs-3", "3"),
			},
			wantName: "rs-2",
			wantRev:  2,
		},
		{
			name:    "SkipsZeroAndNegative",
			current: "3",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-0", "0"),
				rsWithRevision("rs-neg", "-1"),
				rsWithRevision("rs-2", "2"),
			},
			wantName: "rs-2",
			wantRev:  2,
		},
		{
			name:        "OnlyCurrentExists",
			current:     "1",
			replicaSets: []appsv1.ReplicaSet{rsWithRevision("rs-1", "1")},
			wantErr:     "no replicaset with a numeric revision lower than 1",
		},
		{
			name:        "EmptyList",
			current:     "5",
			replicaSets: nil,
			wantErr:     "no replicaset with a numeric revision lower than 5",
		},
		{
			name:    "CurrentMissing_D3",
			current: "",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-1", "1"),
				rsWithRevision("rs-2", "2"),
			},
			wantErr: "is not a positive integer",
		},
		{
			name:    "CurrentNonNumeric_D3",
			current: "x",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-1", "1"),
				rsWithRevision("rs-2", "2"),
			},
			wantErr: "is not a positive integer",
		},
		{
			name:    "LargeRevisionNumbers",
			current: "100000",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-99999", "99999"),
				rsWithRevision("rs-100000", "100000"),
			},
			wantName: "rs-99999",
			wantRev:  99999,
		},
		{
			name:    "DuplicateRevision_FirstWins",
			current: "5",
			replicaSets: []appsv1.ReplicaSet{
				rsWithRevision("rs-a", "4"),
				rsWithRevision("rs-b", "4"),
			},
			wantName: "rs-a",
			wantRev:  4,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotRev, err := selectPreviousReplicaSet(tc.current, tc.replicaSets)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got name=%q rev=%d", tc.wantErr, gotName, gotRev)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				if gotName != "" || gotRev != 0 {
					t.Fatalf("expected empty result on error, got name=%q rev=%d", gotName, gotRev)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotName != tc.wantName || gotRev != tc.wantRev {
				t.Fatalf("got name=%q rev=%d, want name=%q rev=%d", gotName, gotRev, tc.wantName, tc.wantRev)
			}
		})
	}
}
