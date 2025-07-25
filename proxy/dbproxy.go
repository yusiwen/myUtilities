package proxy

import (
	"fmt"
	"github.com/yusiwen/myUtilities/core/proxy"
	"github.com/yusiwen/myUtilities/core/proxy/db"
	"sort"
	"time"
)

func (o *DBProxyOptions) Run() error {
	p, err := o.parseOptions()
	if err != nil {
		return err
	}
	err = p.Start()
	if err != nil {
		return err
	}
	defer p.Close()
	return nil
}

func (o *DBProxyOptions) parseOptions() (*db.OracleProxy, error) {
	backends, err := o.getBackends()
	if err != nil {
		return nil, err
	}
	p := &db.OracleProxy{
		DefaultProxy: proxy.DefaultProxy{
			ListenAddr: getListenAddr(o.Host, o.Port),
		},
		Backends: backends,
	}
	p.HealthCheck.Query = o.DbTestQuery
	p.HealthCheck.Expected = o.DbTestExpected
	p.HealthCheck.Timeout = time.Duration(o.DbTestTimeout) * time.Second
	p.HealthCheck.Interval = time.Duration(o.DbTestInterval) * time.Second

	return p, nil
}

func (o *DBProxyOptions) getBackends() ([]*db.OracleBackendStatus, error) {
	var backends []*db.OracleBackendStatus
	for i, host := range o.DbHost {
		backends = append(backends, &db.OracleBackendStatus{
			Config: db.OracleBackendConfig{
				BackendConfig: proxy.BackendConfig{
					Name:     o.RouteName[i],
					Host:     host,
					Port:     o.DbPort[i],
					Priority: o.RoutePriority[i],
				},
				Username:    o.DbUsername,
				Password:    o.DbPassword,
				ServiceName: o.DbName,
			},
		})
	}
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Config.Priority < backends[j].Config.Priority
	})
	return backends, nil
}

func getListenAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
