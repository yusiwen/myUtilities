package collector

import (
	"github.com/rcrowley/go-metrics"
)

type Collector interface {
	Name() string
	Collect(r metrics.Registry) error
}
