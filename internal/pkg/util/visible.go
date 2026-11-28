/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2026 Hygon Information Technology Co., Ltd.
 */

package util

import (
	"os"
	"strconv"
	"strings"

	"github.com/HYGON-AI/hcu-dcgm/v3/pkg/dcgm"
)

var visibleDevicesFlag string

// SetVisibleDevices sets the CLI/ENV-configured visible devices expression.
func SetVisibleDevices(visible string) {
	visibleDevicesFlag = strings.TrimSpace(visible)
}

// visibleDevicesEnvName returns the single environment variable holding the
// visible-devices whitelist, derived from RESOURCE_PREFIX: HCU_VISIBLE_DEVICES
// for the default hcu prefix, DCU_VISIBLE_DEVICES for dcu, and so on.
func visibleDevicesEnvName() string {
	return strings.ToUpper(ResourcePrefix()) + "_VISIBLE_DEVICES"
}

func resolveVisibleSpec() string {
	if visibleDevicesFlag != "" {
		return visibleDevicesFlag
	}
	return strings.TrimSpace(os.Getenv(visibleDevicesEnvName()))
}

func normalizePCIAddress(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	return strings.TrimPrefix(addr, "0000:")
}

// visibleSet is a parsed whitelist of device indices (DvInd) or PCI bus IDs.
// An empty spec or "all" matches everything; "none" matches nothing.
type visibleSet struct {
	allowAll bool
	indices  map[int]struct{}
	pcis     map[string]struct{}
}

func parseVisibleSpec(spec string) visibleSet {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "all") {
		return visibleSet{allowAll: true}
	}
	if strings.EqualFold(spec, "none") {
		return visibleSet{}
	}

	set := visibleSet{
		indices: make(map[int]struct{}),
		pcis:    make(map[string]struct{}),
	}
	for _, raw := range strings.Split(spec, ",") {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		if idx, err := strconv.Atoi(token); err == nil {
			set.indices[idx] = struct{}{}
			continue
		}
		set.pcis[normalizePCIAddress(token)] = struct{}{}
	}
	return set
}

// allows reports whether a device with the given index and PCI bus ID is in
// the whitelist.
func (s visibleSet) allows(index int, pci string) bool {
	if s.allowAll {
		return true
	}
	if _, ok := s.indices[index]; ok {
		return true
	}
	if _, ok := s.pcis[normalizePCIAddress(pci)]; ok {
		return true
	}
	return false
}

// filterPhysicalDevices filters physical devices by a comma-separated whitelist of
// device indices (DvInd, e.g. "0,1") or PCI bus IDs (e.g. "0000:06:00.0" or "06:00.0").
// Empty spec or "all" keeps all devices; "none" keeps none.
func filterPhysicalDevices(devices map[string]dcgm.DeviceInfo, spec string) map[string]dcgm.DeviceInfo {
	set := parseVisibleSpec(spec)
	filtered := make(map[string]dcgm.DeviceInfo, len(devices))
	for busID, dev := range devices {
		if set.allows(dev.DvInd, busID) {
			filtered[busID] = dev
		}
	}
	return filtered
}

// visiblePhysicalIndices returns the DvInd set of the already-filtered physical
// devices, for filtering derived devices (vHCU/MIG) by their parent card.
func visiblePhysicalIndices(visible map[string]dcgm.DeviceInfo) map[int]struct{} {
	indices := make(map[int]struct{}, len(visible))
	for _, dev := range visible {
		indices[dev.DvInd] = struct{}{}
	}
	return indices
}

// filterVirtualHCUs drops vHCUs whose parent physical card (VDeviceInfo.DvInd,
// per hcu-dcgm v3.0.2) is not in the visible set, and drops the share-* groups
// left empty. The whitelist itself is resolved once on the physical devices, so
// index and PCI-bus specs behave identically here without assuming anything
// about VDeviceInfo.PciBusNumber.
func filterVirtualHCUs(devices map[string][]dcgm.VDeviceInfo, visible map[string]dcgm.DeviceInfo) map[string][]dcgm.VDeviceInfo {
	allowed := visiblePhysicalIndices(visible)
	filtered := make(map[string][]dcgm.VDeviceInfo, len(devices))
	for key, list := range devices {
		kept := make([]dcgm.VDeviceInfo, 0, len(list))
		for _, v := range list {
			if _, ok := allowed[v.DvInd]; ok {
				kept = append(kept, v)
			}
		}
		if len(kept) > 0 {
			filtered[key] = kept
		}
	}
	return filtered
}

// filterMigHCUs drops MIG slices whose parent physical card (MigInfo.DvInd)
// is not in the visible set, and drops the mig-* groups left empty.
func filterMigHCUs(devices map[string][]dcgm.MigInfo, visible map[string]dcgm.DeviceInfo) map[string][]dcgm.MigInfo {
	allowed := visiblePhysicalIndices(visible)
	filtered := make(map[string][]dcgm.MigInfo, len(devices))
	for key, list := range devices {
		kept := make([]dcgm.MigInfo, 0, len(list))
		for _, m := range list {
			if _, ok := allowed[m.DvInd]; ok {
				kept = append(kept, m)
			}
		}
		if len(kept) > 0 {
			filtered[key] = kept
		}
	}
	return filtered
}
