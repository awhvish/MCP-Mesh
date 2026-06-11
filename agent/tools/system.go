package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

func (r *Registry) getSystemInfo(_ map[string]any) (string, error) {
	cpuPct, err := cpu.Percent(0, false)
	if err != nil {
		return "", fmt.Errorf("cpu: %w", err)
	}

	vm, err := mem.VirtualMemory()
	if err != nil {
		return "", fmt.Errorf("memory: %w", err)
	}

	du, err := disk.Usage("/")
	if err != nil {
		return "", fmt.Errorf("disk: %w", err)
	}

	result := map[string]any{
		"cpu_percent":      cpuPct[0],
		"memory_total_mb":  vm.Total / 1024 / 1024,
		"memory_used_mb":   vm.Used / 1024 / 1024,
		"memory_percent":   vm.UsedPercent,
		"disk_total_gb":    du.Total / 1024 / 1024 / 1024,
		"disk_used_gb":     du.Used / 1024 / 1024 / 1024,
		"disk_percent":     du.UsedPercent,
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("encoding result: %w", err)
	}
	return string(out), nil
}

func (r *Registry) shutdownSystem(_ map[string]any) (string, error) {
	if err := exec.Command("systemctl", "poweroff").Run(); err != nil {
		return "", fmt.Errorf("shutdown failed: %w", err)
	}
	return "shutdown initiated", nil
}
