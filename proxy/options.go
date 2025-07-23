package proxy

type DBProxyOptions struct {
	Host           string   `help:"Host to listen on." default:"localhost"`
	Port           int      `help:"Port to listen on." default:"1521"`
	Mode           string   `help:"Mode of database" default:"oracle"`
	RouteName      []string `help:"Name of route" default:""`
	RoutePriority  []int    `help:"Priority of route" default:"0"`
	DbHost         []string `help:"Host of database" default:""`
	DbPort         []int    `help:"Port of database" default:"1521"`
	DbName         []string `help:"Name of database" default:""`
	DbUsername     []string `help:"User name to connect to database" default:""`
	DbPassword     []string `help:"Password to connect to database" default:""`
	DbTestQuery    string   `help:"SQL query statement to test connection" default:"SELECT '1' FROM DUAL"`
	DbTestExpected string   `help:"Expected result of SQL query statement to test connection" default:"1"`
	DbTestTimeout  int      `help:"Timeout in seconds for health check." default:"5"`
	DbTestInterval int      `help:"Interval in seconds for health check." default:"10"`
}

type Options struct {
	DBProxy DBProxyOptions `cmd:"" name:"db" help:"Start a database proxy."`
}
