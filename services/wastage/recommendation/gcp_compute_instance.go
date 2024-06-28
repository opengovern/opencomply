package recommendation

import (
	"context"
	"fmt"
	"github.com/kaytu-io/kaytu-engine/services/wastage/api/entity"
	"github.com/kaytu-io/kaytu-engine/services/wastage/db/model"
	"github.com/kaytu-io/kaytu-engine/services/wastage/recommendation/preferences/gcp_compute"
	"go.uber.org/zap"
	"regexp"
	"strconv"
	"strings"
)

func (s *Service) GCPComputeInstanceRecommendation(
	ctx context.Context,
	instance entity.GcpComputeInstance,
	metrics map[string][]entity.Datapoint,
	preferences map[string]*string,
) (*entity.GcpComputeInstanceRightsizingRecommendation, *model.GCPComputeMachineType, *model.GCPComputeMachineType, error) {
	var machine *model.GCPComputeMachineType
	var err error

	if instance.MachineType == "" {
		return nil, nil, nil, fmt.Errorf("no machine type provided")
	}
	if strings.Contains(instance.MachineType, "custom") {
		machine, err = s.extractCustomInstanceDetails(instance)
	} else {
		machine, err = s.gcpComputeMachineTypeRepo.Get(instance.MachineType)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	currentCost, err := s.costSvc.GetGCPComputeInstanceCost(ctx, instance)
	if err != nil {
		return nil, nil, nil, err
	}

	region := strings.Join([]string{strings.Split(instance.Zone, "-")[0], strings.Split(instance.Zone, "-")[1]}, "-")

	result := entity.GcpComputeInstanceRightsizingRecommendation{
		Current: entity.RightsizingGcpComputeInstance{
			Zone:          instance.Zone,
			Region:        region,
			MachineType:   instance.MachineType,
			MachineFamily: machine.MachineFamily,
			CPU:           machine.GuestCpus,
			MemoryMb:      machine.MemoryMb,

			Cost: currentCost,
		},
	}

	if _, ok := metrics["cpuUtilization"]; !ok {
		return nil, nil, nil, fmt.Errorf("cpuUtilization metric not found")
	}
	if _, ok := metrics["memoryUtilization"]; !ok {
		return nil, nil, nil, fmt.Errorf("memoryUtilization metric not found")
	}
	cpuUsage := extractGCPUsage(metrics["cpuUtilization"])
	memoryUsage := extractGCPUsage(metrics["memoryUtilization"])

	result.CPU = cpuUsage
	result.Memory = memoryUsage

	vCPU := machine.GuestCpus
	cpuBreathingRoom := int64(0)
	if preferences["CPUBreathingRoom"] != nil {
		cpuBreathingRoom, _ = strconv.ParseInt(*preferences["CPUBreathingRoom"], 10, 64)
	}
	memoryBreathingRoom := int64(0)
	if preferences["MemoryBreathingRoom"] != nil {
		memoryBreathingRoom, _ = strconv.ParseInt(*preferences["MemoryBreathingRoom"], 10, 64)
	}
	neededCPU := float64(vCPU) * (getValueOrZero(cpuUsage.Avg) + (float64(cpuBreathingRoom) / 100.0))
	if neededCPU < 2 {
		neededCPU = 2
	}

	neededMemoryMb := 0.0
	if memoryUsage.Avg != nil {
		neededMemoryMb = calculateHeadroom(*memoryUsage.Avg/(1024*1024), memoryBreathingRoom)
	}
	if neededMemoryMb < 1024 {
		neededMemoryMb = 1024
	}

	pref := make(map[string]any)

	for k, v := range preferences {
		var vl any
		if v == nil {
			vl = extractFromGCPComputeInstance(region, machine, k)
		} else {
			vl = *v
		}
		if _, ok := gcp_compute.PreferenceInstanceKey[k]; !ok {
			continue
		}

		cond := "="
		if sc, ok := gcp_compute.PreferenceInstanceSpecialCond[k]; ok {
			cond = sc
		}
		if k == "MemoryGB" {
			vl = int64(vl.(float64) * 1024)
		}
		if k == "MachineFamily" {
			if vl == "custom" {
				continue
			}
		}
		pref[fmt.Sprintf("%s %s ?", gcp_compute.PreferenceInstanceKey[k], cond)] = vl
	}

	suggestedMachineType, err := s.gcpComputeMachineTypeRepo.GetCheapestByCoreAndMemory(neededCPU, neededMemoryMb, pref)
	if err != nil {
		return nil, nil, nil, err
	}

	excludeCustom := false
	if preferences["ExcludeCustomInstances"] != nil {
		if *preferences["ExcludeCustomInstances"] == "Yes" {
			excludeCustom = true
		}
	}

	if suggestedMachineType != nil {
		instance.Zone = suggestedMachineType.Zone
		instance.MachineType = suggestedMachineType.Name
		suggestedCost, err := s.costSvc.GetGCPComputeInstanceCost(ctx, instance)
		if err != nil {
			return nil, nil, nil, err
		}

		if !excludeCustom {
			customMachines, err := s.checkCustomMachines(ctx, region, int64(neededCPU), int64(neededMemoryMb), preferences)
			if err != nil {
				return nil, nil, nil, err
			}
			for _, customMachine := range customMachines {
				if customMachine.Cost < suggestedCost {
					suggestedMachineType = &customMachine.MachineType
					suggestedCost = customMachine.Cost
				}
			}
		}

		result.Recommended = &entity.RightsizingGcpComputeInstance{
			Zone:          suggestedMachineType.Zone,
			Region:        suggestedMachineType.Region,
			MachineType:   suggestedMachineType.Name,
			MachineFamily: suggestedMachineType.MachineFamily,
			CPU:           suggestedMachineType.GuestCpus,
			MemoryMb:      suggestedMachineType.MemoryMb,

			Cost: suggestedCost,
		}
	} else if !excludeCustom {
		customMachines, err := s.checkCustomMachines(ctx, region, int64(neededCPU), int64(neededMemoryMb), preferences)
		if err != nil {
			return nil, nil, nil, err
		}
		suggestedMachineType = machine
		suggestedCost := currentCost

		for _, customMachine := range customMachines {
			if customMachine.Cost < suggestedCost {
				suggestedMachineType = &customMachine.MachineType
				suggestedCost = customMachine.Cost
			}
		}

		result.Recommended = &entity.RightsizingGcpComputeInstance{
			Zone:          suggestedMachineType.Zone,
			Region:        suggestedMachineType.Region,
			MachineType:   suggestedMachineType.Name,
			MachineFamily: suggestedMachineType.MachineFamily,
			CPU:           suggestedMachineType.GuestCpus,
			MemoryMb:      suggestedMachineType.MemoryMb,

			Cost: suggestedCost,
		}
	}
	if suggestedMachineType == nil {
		suggestedMachineType = machine
	}

	return &result, machine, suggestedMachineType, nil
}

func (s *Service) GCPComputeDiskRecommendation(
	ctx context.Context,
	disk entity.GcpComputeDisk,
	currentMachine *model.GCPComputeMachineType,
	recommendedMachine *model.GCPComputeMachineType,
	metrics map[string][]entity.Datapoint,
	preferences map[string]*string,
) (*entity.GcpComputeDiskRecommendation, error) {
	currentCost, err := s.costSvc.GetGCPComputeDiskCost(ctx, disk)
	if err != nil {
		return nil, err
	}

	readIopsUsage := extractGCPUsage(metrics["DiskReadIOPS"])
	writeIopsUsage := extractGCPUsage(metrics["DiskWriteIOPS"])
	readThroughputUsageBytes := extractGCPUsage(metrics["DiskReadThroughput"])
	readThroughputUsageMb := entity.Usage{
		Avg: funcP(readThroughputUsageBytes.Avg, readThroughputUsageBytes.Avg, func(a, _ float64) float64 { return a / (1024 * 1024) }),
		Min: funcP(readThroughputUsageBytes.Min, readThroughputUsageBytes.Min, func(a, _ float64) float64 { return a / (1024 * 1024) }),
		Max: funcP(readThroughputUsageBytes.Max, readThroughputUsageBytes.Max, func(a, _ float64) float64 { return a / (1024 * 1024) }),
	}
	writeThroughputUsageBytes := extractGCPUsage(metrics["DiskWriteThroughput"])
	writeThroughputUsageMb := entity.Usage{
		Avg: funcP(writeThroughputUsageBytes.Avg, writeThroughputUsageBytes.Avg, func(a, _ float64) float64 { return a / (1024 * 1024) }),
		Min: funcP(writeThroughputUsageBytes.Min, writeThroughputUsageBytes.Min, func(a, _ float64) float64 { return a / (1024 * 1024) }),
		Max: funcP(writeThroughputUsageBytes.Max, writeThroughputUsageBytes.Max, func(a, _ float64) float64 { return a / (1024 * 1024) }),
	}

	readIopsLimit, writeIopsLimit, readThroughputLimit, writeThroughputLimit, err := s.getMaximums(currentMachine.MachineFamily,
		currentMachine.MachineType, disk.DiskType, currentMachine.GuestCpus, *disk.DiskSize)
	if err != nil {
		return nil, err
	}

	result := entity.GcpComputeDiskRecommendation{
		Current: entity.RightsizingGcpComputeDisk{
			DiskType:             disk.DiskType,
			DiskSize:             *disk.DiskSize,
			ReadIopsLimit:        readIopsLimit,
			WriteIopsLimit:       writeIopsLimit,
			ReadThroughputLimit:  readThroughputLimit,
			WriteThroughputLimit: writeThroughputLimit,

			Zone:   disk.Zone,
			Region: disk.Region,

			Cost: currentCost,
		},
		ReadIops:        readIopsUsage,
		WriteIops:       writeIopsUsage,
		ReadThroughput:  readThroughputUsageMb,
		WriteThroughput: writeThroughputUsageMb,
	}

	iopsBreathingRoom := int64(0)
	if preferences["IOPSBreathingRoom"] != nil {
		iopsBreathingRoom, _ = strconv.ParseInt(*preferences["IopsBreathingRoom"], 10, 64)
	}

	throughputBreathingRoom := int64(0)
	if preferences["ThroughputBreathingRoom"] != nil {
		throughputBreathingRoom, _ = strconv.ParseInt(*preferences["ThroughputBreathingRoom"], 10, 64)
	}

	neededReadIops := pCalculateHeadroom(readIopsUsage.Avg, iopsBreathingRoom)
	neededReadThroughput := pCalculateHeadroom(readThroughputUsageMb.Avg, throughputBreathingRoom)
	neededWriteIops := pCalculateHeadroom(writeIopsUsage.Avg, iopsBreathingRoom)
	neededWriteThroughput := pCalculateHeadroom(writeThroughputUsageMb.Avg, throughputBreathingRoom)

	pref := make(map[string]any)

	diskSize := *disk.DiskSize
	if ds, ok := preferences["DiskSizeGb"]; ok {
		if ds != nil {
			diskSize, _ = strconv.ParseInt(*ds, 10, 64)
		}
	}

	suggestions, err := s.findCheapestDiskType(recommendedMachine.MachineFamily, recommendedMachine.MachineType, recommendedMachine.GuestCpus,
		neededReadIops, neededWriteIops, neededReadThroughput, neededWriteThroughput, diskSize)
	if err != nil {
		return nil, err
	}

	var suggestedType *string
	var suggestedSize *int64

	if suggestions != nil {
		for _, sug := range suggestions {
			newDisk := entity.GcpComputeDisk{
				HashedDiskId: disk.HashedDiskId,
				Zone:         disk.Zone,
				Region:       disk.Region,
				DiskType:     sug.Type,
				DiskSize:     &sug.Size,
			}
			suggestedCost, err := s.costSvc.GetGCPComputeDiskCost(ctx, newDisk)
			if err != nil {
				return nil, err
			}
			sug.Cost = &suggestedCost
		}
		minPriceSuggestion := suggestions[0]
		for _, sug := range suggestions {
			if _, ok := preferences["DiskSizeGb"]; ok {
				if diskSize != minPriceSuggestion.Size {
					continue
				}
			}
			if *sug.Cost < *minPriceSuggestion.Cost {
				minPriceSuggestion = sug
			}
		}
		suggestedType = &minPriceSuggestion.Type
		suggestedSize = &minPriceSuggestion.Size
	}

	if suggestedType == nil && suggestedSize == nil {
		suggestedType = &disk.DiskType
		suggestedSize = disk.DiskSize
	}

	pref["storage_type = ?"] = suggestedType

	for k, v := range preferences {
		var vl any
		if v == nil {
			vl = extractFromGCPComputeDisk(disk, k)
		} else {
			vl = *v
		}
		if _, ok := gcp_compute.PreferenceDiskKey[k]; !ok {
			continue
		}

		cond := "="

		pref[fmt.Sprintf("%s %s ?", gcp_compute.PreferenceDiskKey[k], cond)] = vl
	}

	suggestedStorageType, err := s.gcpComputeDiskTypeRepo.GetCheapest(pref)
	if err != nil {
		return nil, err
	}

	if suggestedStorageType != nil {
		disk.Zone = suggestedStorageType.Zone
		disk.DiskType = *suggestedType
		disk.Region = suggestedStorageType.Region
		disk.DiskSize = suggestedSize
		suggestedCost, err := s.costSvc.GetGCPComputeDiskCost(ctx, disk)
		if err != nil {
			return nil, err
		}

		recommendedReadIopsLimit, recommendedWriteIopsLimit, recommendedReadThroughputLimit, recommendedWriteThroughputLimit, err := s.getMaximums(recommendedMachine.MachineFamily,
			recommendedMachine.MachineType, suggestedStorageType.StorageType, recommendedMachine.GuestCpus, *disk.DiskSize)
		if err != nil {
			return nil, err
		}

		result.Recommended = &entity.RightsizingGcpComputeDisk{
			Zone:                 suggestedStorageType.Zone,
			Region:               suggestedStorageType.Region,
			DiskType:             suggestedStorageType.StorageType,
			DiskSize:             *disk.DiskSize,
			ReadIopsLimit:        recommendedReadIopsLimit,
			WriteIopsLimit:       recommendedWriteIopsLimit,
			ReadThroughputLimit:  recommendedReadThroughputLimit,
			WriteThroughputLimit: recommendedWriteThroughputLimit,

			Cost: suggestedCost,
		}
	}

	return &result, nil
}

func extractFromGCPComputeInstance(region string, machine *model.GCPComputeMachineType, k string) any {
	switch k {
	case "Region":
		return region
	case "vCPU":
		return machine.GuestCpus
	case "MemoryGB":
		return machine.MemoryMb / 1024
	case "MachineFamily":
		return machine.MachineFamily
	case "MachineType":
		return machine.MachineType
	}
	return ""
}

func extractFromGCPComputeDisk(disk entity.GcpComputeDisk, k string) any {
	switch k {
	case "Region":
		return disk.Region
	case "DiskType":
		return disk.DiskType
	}
	return ""
}

func (s *Service) extractCustomInstanceDetails(instance entity.GcpComputeInstance) (*model.GCPComputeMachineType, error) {
	re := regexp.MustCompile(`(\D.+)-(\d+)-(\d.+)`)
	machineTypePrefix := re.ReplaceAllString(instance.MachineType, "$1")
	strCPUAmount := re.ReplaceAllString(instance.MachineType, "$2")
	strRAMAmount := re.ReplaceAllString(instance.MachineType, "$3")

	region := strings.Join([]string{strings.Split(instance.Zone, "-")[0], strings.Split(instance.Zone, "-")[1]}, "-")
	cpu, err := strconv.ParseInt(strCPUAmount, 10, 64)
	if err != nil {
		return nil, err
	}
	memoryMb, err := strconv.ParseInt(strRAMAmount, 10, 64)
	if err != nil {
		return nil, err
	}

	family := "custom"
	if machineTypePrefix != "custom" {
		family = strings.Split(machineTypePrefix, "-")[0]
	}

	if family == "e2" {
		return nil, fmt.Errorf("e2 instances are not supported")
	}

	return &model.GCPComputeMachineType{
		Name:          instance.MachineType,
		MachineType:   instance.MachineType,
		MachineFamily: family,
		GuestCpus:     cpu,
		MemoryMb:      memoryMb,
		Zone:          instance.Zone,
		Region:        region,
		Description:   "",
		ImageSpaceGb:  0,

		UnitPrice: 0,
	}, nil
}

func (s *Service) checkCustomMachines(ctx context.Context, region string, neededCpu, neededMemoryMb int64, preferences map[string]*string) ([]CustomOffer, error) {
	if preferences["MemoryGB"] != nil && *preferences["MemoryGB"] != "" {
		neededMemoryGb, _ := strconv.ParseInt(*preferences["MemoryGB"], 10, 64)
		neededMemoryMb = neededMemoryGb * 1024
	}
	if preferences["vCPU"] != nil && *preferences["vCPU"] != "" {
		neededCpu, _ = strconv.ParseInt(*preferences["vCPU"], 10, 64)
	}

	offers := make([]CustomOffer, 0)
	if preferences["MachineFamily"] != nil && *preferences["MachineFamily"] != "" {
		offer, err := s.checkCustomMachineForFamily(ctx, region, *preferences["MachineFamily"], neededCpu, neededMemoryMb, preferences)
		if err != nil {
			return nil, err
		}
		if offer == nil {
			return nil, fmt.Errorf("machine family does not have any custom machines")
		}
		return offer, nil
	}

	if neededCpu <= 128 && neededMemoryMb <= 665600 {
		n2Offer, err := s.checkCustomMachineForFamily(ctx, region, "n2", neededCpu, neededMemoryMb, preferences)
		if err != nil {
			return nil, err
		}
		offers = append(offers, n2Offer...)
	}
	if neededCpu <= 80 && neededMemoryMb <= 665600 {
		n4Offer, err := s.checkCustomMachineForFamily(ctx, region, "n4", neededCpu, neededMemoryMb, preferences)
		if err != nil {
			return nil, err
		}
		offers = append(offers, n4Offer...)
	}
	if neededCpu <= 224 && neededMemoryMb <= 786432 {
		n2dOffer, err := s.checkCustomMachineForFamily(ctx, region, "n2d", neededCpu, neededMemoryMb, preferences)
		if err != nil {
			return nil, err
		}
		offers = append(offers, n2dOffer...)
	}
	// TODO: add e2 custom machines
	g2Offer, err := s.checkCustomMachineForFamily(ctx, region, "g2", neededCpu, neededMemoryMb, preferences)
	if err != nil {
		return nil, err
	}
	offers = append(offers, g2Offer...)

	s.logger.Info("custom machines", zap.Any("offers", offers))
	for _, offer := range offers {
		s.logger.Info("custom machine info", zap.String("family", offer.Family), zap.Any("machineType", offer.MachineType), zap.Float64("cost", offer.Cost))
	}

	return offers, nil
}

func (s *Service) checkCustomMachineForFamily(ctx context.Context, region, family string, neededCpu, neededMemoryMb int64, preferences map[string]*string) ([]CustomOffer, error) {
	if neededCpu > 2 {
		neededCpu = roundUpToMultipleOf(neededCpu, 4)
	}
	if family == "n2" || family == "n2d" {
		neededMemoryMb = roundUpToMultipleOf(neededMemoryMb, 256)
		if neededMemoryMb < neededCpu*512 {
			neededMemoryMb = neededCpu * 512
		}
	} else if family == "n4" {
		neededMemoryMb = roundUpToMultipleOf(neededMemoryMb, 256)
		if neededMemoryMb < neededCpu*2048 {
			neededMemoryMb = neededCpu * 2048
		}
	} else if family == "g2" {
		neededMemoryMb = roundUpToMultipleOf(neededMemoryMb, 1024)
		if neededMemoryMb < neededCpu*4096 {
			neededMemoryMb = neededCpu * 4096
		}
	}

	if neededMemoryMb > 8192*neededCpu {
		neededCpu = roundUpToMultipleOf(neededMemoryMb, 8192) / 8192
		neededCpu = roundUpToMultipleOf(neededCpu, 4)
	}

	pref := make(map[string]any)
	for k, v := range preferences {
		if k == "Region" {
			if v != nil && *v != "" {
				pref["location = ?"] = *v
			} else {
				pref["location = ?"] = region
			}
		}
	}

	var customOffers []CustomOffer
	cpuSku, err := s.gcpComputeSKURepo.GetCheapestCustomCore(family, pref)
	if err != nil {
		return nil, err
	}
	if cpuSku == nil {
		return nil, nil
	}
	memorySku, err := s.gcpComputeSKURepo.GetCheapestCustomRam(family, pref)
	if err != nil {
		return nil, err
	}
	if memorySku == nil {
		return nil, nil
	}

	machineType := fmt.Sprintf("%s-custom-%d-%d", family, neededCpu, neededMemoryMb)

	if memorySku.Location == cpuSku.Location {
		cost, err := s.costSvc.GetGCPComputeInstanceCost(ctx, entity.GcpComputeInstance{
			HashedInstanceId: "",
			Zone:             cpuSku.Location + "-a",
			MachineType:      machineType,
		})
		if err != nil {
			return nil, err
		}

		return []CustomOffer{{
			Family: family,
			MachineType: model.GCPComputeMachineType{
				Name:        machineType,
				MachineType: machineType,
				GuestCpus:   neededCpu,
				MemoryMb:    neededMemoryMb,
				Zone:        cpuSku.Location + "-a",
				Region:      cpuSku.Location,
			},
			Cost: cost,
		}}, nil
	}

	cpuRegionCost, err := s.costSvc.GetGCPComputeInstanceCost(ctx, entity.GcpComputeInstance{
		HashedInstanceId: "",
		Zone:             cpuSku.Location + "-a",
		MachineType:      machineType,
	})
	if err != nil {
		return nil, err
	}

	customOffers = append(customOffers, CustomOffer{
		Family: family,
		MachineType: model.GCPComputeMachineType{
			Name:        machineType,
			MachineType: machineType,
			GuestCpus:   neededCpu,
			MemoryMb:    neededMemoryMb,
			Zone:        cpuSku.Location + "-a",
			Region:      cpuSku.Location,
		},
		Cost: cpuRegionCost,
	})

	memoryRegionCost, err := s.costSvc.GetGCPComputeInstanceCost(ctx, entity.GcpComputeInstance{
		HashedInstanceId: "",
		Zone:             memorySku.Location + "-a",
		MachineType:      machineType,
	})
	if err != nil {
		return nil, err
	}

	customOffers = append(customOffers, CustomOffer{
		Family: family,
		MachineType: model.GCPComputeMachineType{
			Name:        machineType,
			MachineType: machineType,
			GuestCpus:   neededCpu,
			MemoryMb:    neededMemoryMb,
			Zone:        memorySku.Location + "-a",
			Region:      memorySku.Location,
		},
		Cost: memoryRegionCost,
	})

	return customOffers, nil
}

type CustomOffer struct {
	Family      string
	MachineType model.GCPComputeMachineType
	Cost        float64
}

func roundUpToMultipleOf(number, multipleOf int64) int64 {
	if number%multipleOf == 0 {
		return number
	}
	return ((number / multipleOf) + 1) * multipleOf
}
