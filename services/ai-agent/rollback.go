package main

import (
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
)

// deploymentRevisionAnnotation is the annotation Kubernetes sets on
// Deployments and their ReplicaSets to record the rollout revision.
const deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"

// selectPreviousReplicaSet returns the name and revision of the ReplicaSet
// with the highest numeric revision that is strictly lower than
// currentRevision.
//
// Revisions are compared as integers, not strings. ReplicaSets whose revision
// annotation is missing, non-numeric, or not a positive integer are ignored.
// If several ReplicaSets share the selected revision, the first one in the
// input order is returned. An error is returned when currentRevision is not a
// positive integer or when no ReplicaSet qualifies.
func selectPreviousReplicaSet(currentRevision string, replicaSets []appsv1.ReplicaSet) (string, int64, error) {
	current, err := strconv.ParseInt(currentRevision, 10, 64)
	if err != nil || current <= 0 {
		return "", 0, fmt.Errorf("current revision %q is not a positive integer", currentRevision)
	}

	var (
		bestName string
		bestRev  int64
	)
	for i := range replicaSets {
		rs := &replicaSets[i]
		raw, ok := rs.Annotations[deploymentRevisionAnnotation]
		if !ok || raw == "" {
			continue
		}
		rev, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || rev <= 0 {
			continue
		}
		if rev >= current {
			continue
		}
		if rev > bestRev {
			bestRev = rev
			bestName = rs.Name
		}
	}

	if bestName == "" {
		return "", 0, fmt.Errorf("no replicaset with a numeric revision lower than %d", current)
	}
	return bestName, bestRev, nil
}
