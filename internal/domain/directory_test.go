package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeDirectorySnapshot(t *testing.T) {
	t.Parallel()

	collectedAt := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	lastSeen := collectedAt.Add(-time.Hour)
	snapshot, normalization, err := NormalizeDirectorySnapshot(DirectorySnapshot{
		CollectedAt: collectedAt,
		Devices: []DirectoryDevice{
			{
				StableNodeID: " node-z ", NodeKey: " nodekey:z ", DNSName: "zeta.example.ts.net.",
				Hostname: " zeta ", OS: "Darwin", TailscaleIPs: []string{"bad", "fd7a:115c:a1e0::2", "100.64.0.2", "100.64.0.2"},
				Tags: []string{"tag:z", "tag:a", "tag:z", ""}, LastSeen: &lastSeen,
			},
			{
				StableNodeID: "node-a", DNSName: "alpha.example.ts.net", OS: "FreeBSD",
				ConnectedToControl: true, LastSeen: &lastSeen,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalization.InvalidAddressCount != 1 {
		t.Fatalf("invalid addresses = %d, want 1", normalization.InvalidAddressCount)
	}
	if got := []string{snapshot.Devices[0].StableNodeID, snapshot.Devices[1].StableNodeID}; !reflect.DeepEqual(got, []string{"node-a", "node-z"}) {
		t.Fatalf("stable order = %#v", got)
	}
	alpha := snapshot.Devices[0]
	if alpha.OS != "FreeBSD" || alpha.LastSeen != nil {
		t.Fatalf("connected device = %#v, want unknown OS retained and lastSeen cleared", alpha)
	}
	zeta := snapshot.Devices[1]
	if zeta.OS != "macos" || zeta.NodeKey != "nodekey:z" || zeta.Hostname != "zeta" {
		t.Fatalf("normalized device = %#v", zeta)
	}
	if !reflect.DeepEqual(zeta.TailscaleIPs, []string{"100.64.0.2", "fd7a:115c:a1e0::2"}) {
		t.Fatalf("addresses = %#v", zeta.TailscaleIPs)
	}
	if !reflect.DeepEqual(zeta.Tags, []string{"tag:a", "tag:z"}) {
		t.Fatalf("tags = %#v", zeta.Tags)
	}
}

func TestNormalizeDirectorySnapshotRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	tests := map[string]DirectorySnapshot{
		"missing collected time": {Devices: []DirectoryDevice{}},
		"empty stable ID":        {CollectedAt: now, Devices: []DirectoryDevice{{Hostname: "device"}}},
		"duplicate stable ID": {
			CollectedAt: now,
			Devices: []DirectoryDevice{
				{StableNodeID: "node-a"},
				{StableNodeID: " node-a "},
			},
		},
	}
	for name, snapshot := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := NormalizeDirectorySnapshot(snapshot); err == nil {
				t.Fatal("invalid snapshot was accepted")
			}
		})
	}
}

func TestDirectoryNodeKeyCollisionDoesNotInvalidateSnapshot(t *testing.T) {
	t.Parallel()

	snapshot, _, err := NormalizeDirectorySnapshot(DirectorySnapshot{
		CollectedAt: time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC),
		Devices: []DirectoryDevice{
			{StableNodeID: "node-b", NodeKey: "nodekey:shared"},
			{StableNodeID: "node-a", NodeKey: "nodekey:shared"},
			{StableNodeID: "node-c", NodeKey: "nodekey:unique"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.ConflictingNodeKeys(); !reflect.DeepEqual(got, map[string][]string{
		"nodekey:shared": {"node-a", "node-b"},
	}) {
		t.Fatalf("node-key conflicts = %#v", got)
	}
}

func TestDirectoryMetadataConflictsNormalizeComparableValues(t *testing.T) {
	t.Parallel()

	directoryAt := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	runtimeAt := directoryAt.Add(-time.Minute)
	directory := DirectoryDevice{
		DNSName: "Device.Example.ts.net.", Hostname: "DEVICE", OS: "macos",
		TailscaleIPs: []string{"fd7a:115c:a1e0::1", "100.64.0.1"},
	}
	runtime := NodeIdentity{
		DNSName: "device.example.ts.net", Hostname: "device.", OS: "darwin",
		TailscaleIPs: []string{"100.64.0.1", "fd7a:115c:a1e0:0:0:0:0:1"},
	}
	if conflicts := DirectoryMetadataConflicts(directory, runtime, directoryAt, runtimeAt); len(conflicts) != 0 {
		t.Fatalf("equivalent values conflicted: %#v", conflicts)
	}

	directory.Hostname = "renamed-device"
	directory.OS = "linux"
	directory.TailscaleIPs = []string{"100.64.0.2"}
	conflicts := DirectoryMetadataConflicts(directory, runtime, directoryAt, runtimeAt)
	if got := conflictFields(conflicts); !reflect.DeepEqual(got, []MetadataField{
		MetadataHostname, MetadataOS, MetadataTailscaleIPs,
	}) {
		t.Fatalf("conflict fields = %#v", got)
	}
	for _, conflict := range conflicts {
		if !conflict.DirectoryCollectedAt.Equal(directoryAt) || !conflict.RuntimeCollectedAt.Equal(runtimeAt) {
			t.Fatalf("conflict times = %#v", conflict)
		}
	}
}

func TestDirectoryMetadataConflictsIgnoreMissingSide(t *testing.T) {
	t.Parallel()

	if conflicts := DirectoryMetadataConflicts(
		DirectoryDevice{DNSName: "directory.example.ts.net"},
		NodeIdentity{Hostname: "runtime"},
		time.Now(), time.Now(),
	); len(conflicts) != 0 {
		t.Fatalf("missing comparable values conflicted: %#v", conflicts)
	}
}

func TestDirectorySnapshotCloneDoesNotAliasSlicesOrTime(t *testing.T) {
	t.Parallel()

	lastSeen := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	original := DirectorySnapshot{Devices: []DirectoryDevice{{
		StableNodeID: "node-a", TailscaleIPs: []string{"100.64.0.1"}, Tags: []string{"tag:a"}, LastSeen: &lastSeen,
	}}}
	clone := original.Clone()
	clone.Devices[0].TailscaleIPs[0] = "100.64.0.2"
	clone.Devices[0].Tags[0] = "tag:b"
	*clone.Devices[0].LastSeen = lastSeen.Add(time.Hour)
	if original.Devices[0].TailscaleIPs[0] != "100.64.0.1" || original.Devices[0].Tags[0] != "tag:a" ||
		!original.Devices[0].LastSeen.Equal(lastSeen) {
		t.Fatalf("clone mutated original: %#v", original)
	}
}

func TestDirectorySyncStateValidation(t *testing.T) {
	t.Parallel()

	attempt := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	success := attempt.Add(-time.Minute)
	retry := attempt.Add(30 * time.Second)
	valid := []DirectorySyncState{
		{Status: DirectorySyncDisabled},
		{Status: DirectorySyncSyncing, LastSuccessAt: &success},
		{Status: DirectorySyncHealthy, LastAttemptAt: &attempt, LastSuccessAt: &attempt},
		{Status: DirectorySyncStale, LastAttemptAt: &attempt, LastSuccessAt: &success, NextRetryAt: &retry, ErrorCode: DirectoryErrorTimeout},
	}
	for _, state := range valid {
		if err := state.Validate(); err != nil {
			t.Fatalf("valid state %#v rejected: %v", state, err)
		}
	}
	invalid := []DirectorySyncState{
		{},
		{Status: DirectorySyncHealthy},
		{Status: DirectorySyncStale, LastAttemptAt: &attempt, NextRetryAt: &retry},
		{Status: DirectorySyncStale, LastAttemptAt: &attempt, NextRetryAt: &attempt, ErrorCode: DirectoryErrorTimeout},
		{Status: DirectorySyncStale, LastAttemptAt: &attempt, NextRetryAt: &retry, ErrorCode: "raw-error"},
		{Status: DirectorySyncDisabled, ErrorCode: DirectoryErrorForbidden},
		{Status: DirectorySyncHealthy, LastSuccessAt: &attempt, InvalidAddressCount: -1},
	}
	for _, state := range invalid {
		if err := state.Validate(); err == nil {
			t.Fatalf("invalid state accepted: %#v", state)
		}
	}
}

func conflictFields(conflicts []MetadataConflict) []MetadataField {
	result := make([]MetadataField, len(conflicts))
	for index, conflict := range conflicts {
		result[index] = conflict.Field
	}
	return result
}
