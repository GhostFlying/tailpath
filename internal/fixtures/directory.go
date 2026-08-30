package fixtures

import (
	"fmt"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

const DefaultDirectoryDeviceCount = 250

func DeviceDirectorySnapshot(at time.Time, count int) domain.DirectorySnapshot {
	at = at.UTC()
	if count < 0 {
		count = 0
	}
	overlap := count * 4 / 5
	devices := make([]domain.DirectoryDevice, 0, count)
	for index := range count {
		device := domain.DirectoryDevice{
			DNSName:  fmt.Sprintf("catalog-node-%03d.directory.example.ts.net.", index+1),
			Hostname: fmt.Sprintf("catalog-node-%03d", index+1),
			OS:       [...]string{"linux", "macos", "windows", "ios", "android"}[index%5],
			Tags:     []string{fmt.Sprintf("tag:group-%d", index%4+1)},
		}
		if index < overlap {
			runtime := scaleNode(index)
			device.StableNodeID = runtime.StableNodeID
			device.NodeKey = runtime.NodeKey
			device.TailscaleIPs = append([]string{}, runtime.TailscaleIPs...)
			if index%17 == 0 {
				device.OS = "catalog-os"
			}
		} else {
			device.StableNodeID = fmt.Sprintf("directory-only-%03d", index-overlap+1)
			device.NodeKey = fmt.Sprintf("nodekey:directory-only-%064x", index+1)
			device.TailscaleIPs = []string{fmt.Sprintf("100.110.%d.%d", index/250, index%250+1)}
		}
		if index%7 == 0 {
			device.TailscaleIPs = append(device.TailscaleIPs, fmt.Sprintf("fd7a:115c:a1e0::%x", index+1))
		}
		device.ConnectedToControl = index%3 != 0
		if !device.ConnectedToControl {
			lastSeen := at.Add(-time.Duration(index+1) * time.Minute)
			device.LastSeen = &lastSeen
		}
		if index == count-1 {
			device.OS = "synthetic-os"
		}
		devices = append(devices, device)
	}
	return domain.DirectorySnapshot{CollectedAt: at, Devices: devices}
}
