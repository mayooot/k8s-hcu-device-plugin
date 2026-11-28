/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2026 Hygon Information Technology Co., Ltd.
 */

package util

import (
	"testing"

	"github.com/HYGON-AI/hcu-dcgm/v3/pkg/dcgm"
)

func sampleDevices() map[string]dcgm.DeviceInfo {
	return map[string]dcgm.DeviceInfo{
		"0000:06:00.0": {DvInd: 0, PciBusNumber: "0000:06:00.0"},
		"0000:09:00.0": {DvInd: 1, PciBusNumber: "0000:09:00.0"},
		"0000:0c:00.0": {DvInd: 2, PciBusNumber: "0000:0c:00.0"},
	}
}

func TestFilterPhysicalDevices(t *testing.T) {
	devs := sampleDevices()

	if got := filterPhysicalDevices(devs, ""); len(got) != 3 {
		t.Fatalf("empty spec: want 3, got %d", len(got))
	}
	if got := filterPhysicalDevices(devs, "ALL"); len(got) != 3 {
		t.Fatalf("all spec: want 3, got %d", len(got))
	}
	if got := filterPhysicalDevices(devs, "none"); len(got) != 0 {
		t.Fatalf("none spec: want 0, got %d", len(got))
	}

	gotIdx := filterPhysicalDevices(devs, "0, 2")
	if len(gotIdx) != 2 {
		t.Fatalf("index spec: want 2, got %d", len(gotIdx))
	}
	if _, ok := gotIdx["0000:06:00.0"]; !ok {
		t.Fatalf("expected 0000:06:00.0 in index filter")
	}
	if _, ok := gotIdx["0000:0c:00.0"]; !ok {
		t.Fatalf("expected 0000:0c:00.0 in index filter")
	}

	gotPCI := filterPhysicalDevices(devs, "0000:09:00.0,0C:00.0")
	if len(gotPCI) != 2 {
		t.Fatalf("pci spec: want 2, got %d", len(gotPCI))
	}
	if _, ok := gotPCI["0000:09:00.0"]; !ok {
		t.Fatalf("expected 0000:09:00.0 in pci filter")
	}
	if _, ok := gotPCI["0000:0c:00.0"]; !ok {
		t.Fatalf("expected 0000:0c:00.0 in pci filter")
	}
}

func TestResolveVisibleSpecPriority(t *testing.T) {
	t.Cleanup(func() {
		SetVisibleDevices("")
		_ = InitResourcePrefix(DefaultResourcePrefix)
	})

	SetVisibleDevices("")
	t.Setenv("DCU_VISIBLE_DEVICES", "1,2")
	if got := resolveVisibleSpec(); got != "" {
		t.Fatalf("DCU_VISIBLE_DEVICES under hcu prefix: want empty, got %q", got)
	}

	if err := InitResourcePrefix("dcu"); err != nil {
		t.Fatalf("InitResourcePrefix(dcu): %v", err)
	}
	if got := resolveVisibleSpec(); got != "1,2" {
		t.Fatalf("DCU_VISIBLE_DEVICES under dcu prefix: want 1,2, got %q", got)
	}

	t.Setenv("HCU_VISIBLE_DEVICES", "0")
	if got := resolveVisibleSpec(); got != "1,2" {
		t.Fatalf("HCU_VISIBLE_DEVICES must not win under dcu prefix: want 1,2, got %q", got)
	}

	SetVisibleDevices("0000:06:00.0")
	if got := resolveVisibleSpec(); got != "0000:06:00.0" {
		t.Fatalf("flag precedence: want 0000:06:00.0, got %q", got)
	}
}

func sampleVirtualHCUs() map[string][]dcgm.VDeviceInfo {
	return map[string][]dcgm.VDeviceInfo{
		"share-4c-16g": {
			{DvInd: 0, VdvInd: 0, PciBusNumber: "0000:06:00.0"},
			{DvInd: 1, VdvInd: 1, PciBusNumber: "0000:09:00.0"},
			{DvInd: 2, VdvInd: 2, PciBusNumber: "0000:0c:00.0"},
		},
		"share-2c-8g": {
			{DvInd: 2, VdvInd: 3, PciBusNumber: "0000:0c:00.0"},
		},
	}
}

func TestFilterVirtualHCUs(t *testing.T) {
	visible := filterPhysicalDevices(sampleDevices(), "0,1")

	got := filterVirtualHCUs(sampleVirtualHCUs(), visible)
	if len(got) != 1 {
		t.Fatalf("want 1 group left, got %d", len(got))
	}
	kept, ok := got["share-4c-16g"]
	if !ok {
		t.Fatalf("expected share-4c-16g to survive")
	}
	if len(kept) != 2 {
		t.Fatalf("share-4c-16g: want 2 slices, got %d", len(kept))
	}
	for _, v := range kept {
		if v.DvInd == 2 {
			t.Fatalf("slice of hidden card 2 must not be reported")
		}
	}
	if _, ok := got["share-2c-8g"]; ok {
		t.Fatalf("share-2c-8g has only hidden parents and must be dropped")
	}

	if got := filterVirtualHCUs(sampleVirtualHCUs(), filterPhysicalDevices(sampleDevices(), "none")); len(got) != 0 {
		t.Fatalf("none spec: want 0 groups, got %d", len(got))
	}
	if got := filterVirtualHCUs(sampleVirtualHCUs(), filterPhysicalDevices(sampleDevices(), "")); len(got) != 2 {
		t.Fatalf("empty spec: want 2 groups, got %d", len(got))
	}
}

func sampleMigHCUs() map[string][]dcgm.MigInfo {
	return map[string][]dcgm.MigInfo{
		"mig-1g-5gb": {
			{DvInd: 0, MigId: 0, UUID: "mig-0"},
			{DvInd: 2, MigId: 1, UUID: "mig-2"},
		},
	}
}

func TestFilterMigHCUs(t *testing.T) {
	visible := filterPhysicalDevices(sampleDevices(), "0,1")

	got := filterMigHCUs(sampleMigHCUs(), visible)
	if len(got) != 1 {
		t.Fatalf("want 1 group left, got %d", len(got))
	}
	kept := got["mig-1g-5gb"]
	if len(kept) != 1 || kept[0].DvInd != 0 {
		t.Fatalf("mig-1g-5gb: want only the slice of card 0, got %+v", kept)
	}

	if got := filterMigHCUs(sampleMigHCUs(), filterPhysicalDevices(sampleDevices(), "none")); len(got) != 0 {
		t.Fatalf("none spec: want 0 groups, got %d", len(got))
	}
}
