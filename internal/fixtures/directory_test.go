package fixtures

import (
	"strings"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestDeviceDirectorySnapshotIsDeterministicAndMixed(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	snapshot := DeviceDirectorySnapshot(at, DefaultDirectoryDeviceCount)
	normalized, _, err := domain.NormalizeDirectorySnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.Devices) != DefaultDirectoryDeviceCount {
		t.Fatalf("devices = %d", len(normalized.Devices))
	}
	counts := map[string]int{}
	for _, device := range normalized.Devices {
		if strings.HasPrefix(device.StableNodeID, "scale-") {
			counts["runtime"]++
		} else {
			counts["directory-only"]++
		}
		if device.ConnectedToControl && device.LastSeen != nil {
			t.Fatalf("connected device retained lastSeen: %#v", device)
		}
	}
	if counts["runtime"] != 200 || counts["directory-only"] != 50 {
		t.Fatalf("fixture mix = %#v", counts)
	}
	second, _, err := domain.NormalizeDirectorySnapshot(DeviceDirectorySnapshot(at, DefaultDirectoryDeviceCount))
	if err != nil {
		t.Fatal(err)
	}
	if second.Devices[0].StableNodeID != normalized.Devices[0].StableNodeID ||
		second.Devices[len(second.Devices)-1].StableNodeID != normalized.Devices[len(normalized.Devices)-1].StableNodeID {
		t.Fatal("directory fixture order is not deterministic")
	}
}
