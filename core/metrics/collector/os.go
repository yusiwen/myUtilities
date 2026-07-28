package collector

import (
	"context"
	"fmt"

	"github.com/rcrowley/go-metrics"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

type OSCollector struct{}

func NewOSCollector() *OSCollector {
	return &OSCollector{}
}

func (o *OSCollector) Name() string {
	return "os"
}

func (o *OSCollector) Collect(r metrics.Registry) error {
	ctx := context.Background()

	o.collectCPU(ctx, r)
	o.collectMemory(ctx, r)
	o.collectDisk(ctx, r)
	o.collectNet(ctx, r)
	o.collectLoad(ctx, r)

	return nil
}

func getOrAddGauge(r metrics.Registry, name string) metrics.Gauge {
	if g := r.Get(name); g != nil {
		return g.(metrics.Gauge)
	}
	return metrics.GetOrRegisterGauge(name, r)
}

func getOrAddGaugeFloat64(r metrics.Registry, name string) metrics.GaugeFloat64 {
	if g := r.Get(name); g != nil {
		return g.(metrics.GaugeFloat64)
	}
	return metrics.GetOrRegisterGaugeFloat64(name, r)
}

func (o *OSCollector) collectCPU(ctx context.Context, r metrics.Registry) {
	p, err := cpu.PercentWithContext(ctx, 0, false)
	if err == nil && len(p) > 0 {
		getOrAddGaugeFloat64(r, "cpu.used.percent").Update(p[0])
	}

	perCPU, err := cpu.PercentWithContext(ctx, 0, true)
	if err == nil {
		for i, v := range perCPU {
			getOrAddGaugeFloat64(r, fmt.Sprintf("cpu.per_cpu.percent.cpu%d", i)).Update(v)
		}
	}
}

func (o *OSCollector) collectMemory(ctx context.Context, r metrics.Registry) {
	v, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return
	}
	getOrAddGaugeFloat64(r, "memory.used.percent").Update(v.UsedPercent)
	getOrAddGauge(r, "memory.total.bytes").Update(int64(v.Total))
	getOrAddGauge(r, "memory.used.bytes").Update(int64(v.Used))
	getOrAddGauge(r, "memory.free.bytes").Update(int64(v.Free))
}

func (o *OSCollector) collectDisk(ctx context.Context, r metrics.Registry) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return
	}
	for _, p := range parts {
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue
		}
		name := fmt.Sprintf("disk.used.percent.%s", p.Device)
		getOrAddGaugeFloat64(r, name).Update(usage.UsedPercent)
	}

	ioCounters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return
	}
	for dev, io := range ioCounters {
		getOrAddGauge(r, fmt.Sprintf("disk.io.read_bytes.%s", dev)).Update(int64(io.ReadBytes))
		getOrAddGauge(r, fmt.Sprintf("disk.io.write_bytes.%s", dev)).Update(int64(io.WriteBytes))
	}
}

func (o *OSCollector) collectNet(ctx context.Context, r metrics.Registry) {
	counters, err := net.IOCountersWithContext(ctx, false)
	if err != nil || len(counters) == 0 {
		return
	}
	for _, c := range counters {
		getOrAddGauge(r, fmt.Sprintf("net.io.bytes_in.%s", c.Name)).Update(int64(c.BytesRecv))
		getOrAddGauge(r, fmt.Sprintf("net.io.bytes_out.%s", c.Name)).Update(int64(c.BytesSent))
	}
}

func (o *OSCollector) collectLoad(ctx context.Context, r metrics.Registry) {
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return
	}
	getOrAddGaugeFloat64(r, "load.1m").Update(avg.Load1)
	getOrAddGaugeFloat64(r, "load.5m").Update(avg.Load5)
	getOrAddGaugeFloat64(r, "load.15m").Update(avg.Load15)
}


