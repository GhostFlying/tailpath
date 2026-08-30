package domain

import (
	"errors"
	"net/netip"
	"sort"
	"strings"
	"time"
)

type DirectorySyncStatus string

const (
	DirectorySyncDisabled DirectorySyncStatus = "disabled"
	DirectorySyncSyncing  DirectorySyncStatus = "syncing"
	DirectorySyncHealthy  DirectorySyncStatus = "healthy"
	DirectorySyncStale    DirectorySyncStatus = "stale"
)

type DirectoryErrorCode string

const (
	DirectoryErrorUnauthorized    DirectoryErrorCode = "unauthorized"
	DirectoryErrorForbidden       DirectoryErrorCode = "forbidden"
	DirectoryErrorRateLimited     DirectoryErrorCode = "rate-limited"
	DirectoryErrorUnavailable     DirectoryErrorCode = "unavailable"
	DirectoryErrorTimeout         DirectoryErrorCode = "timeout"
	DirectoryErrorInvalidResponse DirectoryErrorCode = "invalid-response"
)

type DirectorySyncState struct {
	Status              DirectorySyncStatus `json:"status"`
	LastAttemptAt       *time.Time          `json:"lastAttemptAt,omitempty"`
	LastSuccessAt       *time.Time          `json:"lastSuccessAt,omitempty"`
	NextRetryAt         *time.Time          `json:"nextRetryAt,omitempty"`
	ErrorCode           DirectoryErrorCode  `json:"errorCode,omitempty"`
	InvalidAddressCount int                 `json:"invalidAddressCount,omitempty"`
}

func (state DirectorySyncState) Clone() DirectorySyncState {
	result := state
	result.LastAttemptAt = cloneTime(state.LastAttemptAt)
	result.LastSuccessAt = cloneTime(state.LastSuccessAt)
	result.NextRetryAt = cloneTime(state.NextRetryAt)
	return result
}

func (state DirectorySyncState) Validate() error {
	for _, value := range []*time.Time{state.LastAttemptAt, state.LastSuccessAt, state.NextRetryAt} {
		if value != nil && value.IsZero() {
			return errors.New("directory sync timestamps cannot be zero")
		}
	}
	if state.InvalidAddressCount < 0 {
		return errors.New("directory invalid address count cannot be negative")
	}
	switch state.Status {
	case DirectorySyncDisabled:
		if state.ErrorCode != "" || state.NextRetryAt != nil {
			return errors.New("disabled directory sync cannot contain an error or retry")
		}
	case DirectorySyncSyncing:
		if state.ErrorCode != "" || state.NextRetryAt != nil {
			return errors.New("syncing directory state cannot contain an error or retry")
		}
	case DirectorySyncHealthy:
		if state.LastSuccessAt == nil {
			return errors.New("healthy directory sync requires a success time")
		}
		if state.ErrorCode != "" || state.NextRetryAt != nil {
			return errors.New("healthy directory sync cannot contain an error or retry")
		}
	case DirectorySyncStale:
		if state.LastAttemptAt == nil || state.ErrorCode == "" || state.NextRetryAt == nil {
			return errors.New("stale directory sync requires attempt, error, and retry")
		}
		if !validDirectoryErrorCode(state.ErrorCode) {
			return errors.New("stale directory sync contains an unknown error code")
		}
	default:
		return errors.New("unknown directory sync status")
	}
	if state.LastAttemptAt != nil && state.LastSuccessAt != nil && state.LastSuccessAt.After(*state.LastAttemptAt) {
		return errors.New("directory success cannot be later than its last attempt")
	}
	if state.NextRetryAt != nil && state.LastAttemptAt != nil && !state.NextRetryAt.After(*state.LastAttemptAt) {
		return errors.New("directory retry must be later than its last attempt")
	}
	return nil
}

func validDirectoryErrorCode(code DirectoryErrorCode) bool {
	switch code {
	case DirectoryErrorUnauthorized, DirectoryErrorForbidden, DirectoryErrorRateLimited,
		DirectoryErrorUnavailable, DirectoryErrorTimeout, DirectoryErrorInvalidResponse:
		return true
	default:
		return false
	}
}

type DirectorySnapshot struct {
	CollectedAt time.Time         `json:"collectedAt"`
	Devices     []DirectoryDevice `json:"devices"`
}

type DirectoryDevice struct {
	StableNodeID       string     `json:"stableNodeId"`
	NodeKey            string     `json:"nodeKey,omitempty"`
	DNSName            string     `json:"dnsName,omitempty"`
	Hostname           string     `json:"hostname,omitempty"`
	OS                 string     `json:"os,omitempty"`
	TailscaleIPs       []string   `json:"tailscaleIps"`
	Tags               []string   `json:"tags"`
	ConnectedToControl bool       `json:"connectedToControl"`
	LastSeen           *time.Time `json:"lastSeen,omitempty"`
}

func (device DirectoryDevice) Clone() DirectoryDevice {
	result := device
	result.TailscaleIPs = append([]string{}, device.TailscaleIPs...)
	result.Tags = append([]string{}, device.Tags...)
	result.LastSeen = cloneTime(device.LastSeen)
	return result
}

func (device DirectoryDevice) DisplayName() string {
	return NodeIdentity{DNSName: device.DNSName, Hostname: device.Hostname}.DisplayName()
}

type DirectoryNormalization struct {
	InvalidAddressCount int
}

func NormalizeDirectorySnapshot(snapshot DirectorySnapshot) (DirectorySnapshot, DirectoryNormalization, error) {
	if snapshot.CollectedAt.IsZero() {
		return DirectorySnapshot{}, DirectoryNormalization{}, errors.New("directory collectedAt is required")
	}

	result := DirectorySnapshot{
		CollectedAt: snapshot.CollectedAt,
		Devices:     make([]DirectoryDevice, 0, len(snapshot.Devices)),
	}
	normalization := DirectoryNormalization{}
	stableIDs := make(map[string]struct{}, len(snapshot.Devices))
	for _, input := range snapshot.Devices {
		device, invalidAddresses := normalizeDirectoryDevice(input)
		normalization.InvalidAddressCount += invalidAddresses
		if device.StableNodeID == "" {
			return DirectorySnapshot{}, DirectoryNormalization{}, errors.New("directory StableNodeID is required")
		}
		if _, exists := stableIDs[device.StableNodeID]; exists {
			return DirectorySnapshot{}, DirectoryNormalization{}, errors.New("directory StableNodeID must be unique")
		}
		stableIDs[device.StableNodeID] = struct{}{}
		result.Devices = append(result.Devices, device)
	}
	sort.Slice(result.Devices, func(i, j int) bool {
		left := strings.ToLower(result.Devices[i].DisplayName())
		right := strings.ToLower(result.Devices[j].DisplayName())
		if left != right {
			return left < right
		}
		return result.Devices[i].StableNodeID < result.Devices[j].StableNodeID
	})
	return result, normalization, nil
}

func normalizeDirectoryDevice(device DirectoryDevice) (DirectoryDevice, int) {
	result := DirectoryDevice{
		StableNodeID:       strings.TrimSpace(device.StableNodeID),
		NodeKey:            strings.TrimSpace(device.NodeKey),
		DNSName:            strings.TrimSpace(device.DNSName),
		Hostname:           strings.TrimSpace(device.Hostname),
		OS:                 normalizeDirectoryOS(device.OS),
		ConnectedToControl: device.ConnectedToControl,
	}
	result.Tags = normalizedStrings(device.Tags)
	var invalidAddresses int
	result.TailscaleIPs, invalidAddresses = normalizedAddresses(device.TailscaleIPs)
	if !device.ConnectedToControl && device.LastSeen != nil && !device.LastSeen.IsZero() {
		value := *device.LastSeen
		result.LastSeen = &value
	}
	return result, invalidAddresses
}

func normalizeDirectoryOS(value string) string {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "linux":
		return "linux"
	case "darwin", "macos":
		return "macos"
	case "windows":
		return "windows"
	case "ios":
		return "ios"
	case "android":
		return "android"
	default:
		return trimmed
	}
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizedAddresses(values []string) ([]string, int) {
	seen := make(map[netip.Addr]struct{}, len(values))
	addresses := make([]netip.Addr, 0, len(values))
	invalid := 0
	for _, value := range values {
		address, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil {
			invalid++
			continue
		}
		address = address.Unmap()
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Compare(addresses[j]) < 0 })
	result := make([]string, len(addresses))
	for index, address := range addresses {
		result[index] = address.String()
	}
	return result, invalid
}

func (snapshot DirectorySnapshot) Clone() DirectorySnapshot {
	result := DirectorySnapshot{CollectedAt: snapshot.CollectedAt, Devices: make([]DirectoryDevice, len(snapshot.Devices))}
	for index, device := range snapshot.Devices {
		result.Devices[index] = device.Clone()
	}
	return result
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func (snapshot DirectorySnapshot) ConflictingNodeKeys() map[string][]string {
	owners := make(map[string][]string)
	for _, device := range snapshot.Devices {
		if device.NodeKey == "" {
			continue
		}
		owners[device.NodeKey] = append(owners[device.NodeKey], device.StableNodeID)
	}
	for nodeKey, stableIDs := range owners {
		stableIDs = normalizedStrings(stableIDs)
		if len(stableIDs) < 2 {
			delete(owners, nodeKey)
			continue
		}
		owners[nodeKey] = stableIDs
	}
	return owners
}

type MetadataField string

const (
	MetadataDNSName      MetadataField = "dnsName"
	MetadataHostname     MetadataField = "hostname"
	MetadataOS           MetadataField = "os"
	MetadataTailscaleIPs MetadataField = "tailscaleIps"
)

type MetadataConflict struct {
	Field                MetadataField `json:"field"`
	DirectoryValues      []string      `json:"directoryValues"`
	RuntimeValues        []string      `json:"runtimeValues"`
	DirectoryCollectedAt time.Time     `json:"directoryCollectedAt"`
	RuntimeCollectedAt   time.Time     `json:"runtimeCollectedAt"`
}

type DirectoryEnrichment struct {
	StableNodeID       string             `json:"stableNodeId"`
	DNSName            string             `json:"dnsName,omitempty"`
	Hostname           string             `json:"hostname,omitempty"`
	OS                 string             `json:"os,omitempty"`
	TailscaleIPs       []string           `json:"tailscaleIps"`
	Tags               []string           `json:"tags"`
	ConnectedToControl bool               `json:"connectedToControl"`
	LastSeen           *time.Time         `json:"lastSeen,omitempty"`
	CollectedAt        time.Time          `json:"collectedAt"`
	Conflicts          []MetadataConflict `json:"conflicts"`
}

type DirectoryRuntimeEvidence struct {
	Identity       NodeIdentity `json:"identity"`
	Observable     bool         `json:"observable"`
	Online         bool         `json:"online"`
	LastEvidenceAt time.Time    `json:"lastEvidenceAt"`
	CollectedAt    time.Time    `json:"collectedAt"`
}

type DirectoryNode struct {
	ID             string                    `json:"id"`
	Device         DirectoryDevice           `json:"device"`
	CollectedAt    time.Time                 `json:"collectedAt"`
	Runtime        *DirectoryRuntimeEvidence `json:"runtime,omitempty"`
	IdentityStatus IdentityStatus            `json:"identityStatus"`
	Conflicts      []MetadataConflict        `json:"conflicts"`
}

type DeviceDirectory struct {
	Sync    DirectorySyncState `json:"sync"`
	Devices []DirectoryNode    `json:"devices"`
}

func DirectoryMetadataConflicts(directory DirectoryDevice, runtime NodeIdentity, directoryAt, runtimeAt time.Time) []MetadataConflict {
	conflicts := make([]MetadataConflict, 0, 4)
	conflicts = appendMetadataConflict(conflicts, MetadataDNSName,
		[]string{directory.DNSName}, []string{runtime.DNSName}, directoryAt, runtimeAt, normalizeNameValues)
	conflicts = appendMetadataConflict(conflicts, MetadataHostname,
		[]string{directory.Hostname}, []string{runtime.Hostname}, directoryAt, runtimeAt, normalizeNameValues)
	conflicts = appendMetadataConflict(conflicts, MetadataOS,
		[]string{directory.OS}, []string{runtime.OS}, directoryAt, runtimeAt, normalizeOSValues)
	conflicts = appendMetadataConflict(conflicts, MetadataTailscaleIPs,
		directory.TailscaleIPs, runtime.TailscaleIPs, directoryAt, runtimeAt, normalizeAddressValues)
	return conflicts
}

type metadataNormalizer func([]string) []string

func appendMetadataConflict(conflicts []MetadataConflict, field MetadataField, directoryValues, runtimeValues []string,
	directoryAt, runtimeAt time.Time, normalize metadataNormalizer,
) []MetadataConflict {
	directoryNormalized := normalize(directoryValues)
	runtimeNormalized := normalize(runtimeValues)
	if len(directoryNormalized) == 0 || len(runtimeNormalized) == 0 || equalStrings(directoryNormalized, runtimeNormalized) {
		return conflicts
	}
	return append(conflicts, MetadataConflict{
		Field:                field,
		DirectoryValues:      nonEmptyTrimmed(directoryValues),
		RuntimeValues:        nonEmptyTrimmed(runtimeValues),
		DirectoryCollectedAt: directoryAt,
		RuntimeCollectedAt:   runtimeAt,
	})
}

func normalizeNameValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
		if value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func normalizeOSValues(values []string) []string {
	result := nonEmptyTrimmed(values)
	for index := range result {
		result[index] = strings.ToLower(normalizeDirectoryOS(result[index]))
	}
	sort.Strings(result)
	return result
}

func normalizeAddressValues(values []string) []string {
	valid, _ := normalizedAddresses(values)
	return valid
}

func nonEmptyTrimmed(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
