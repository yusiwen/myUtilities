package mock

type FileServerOptions struct {
	LocalDir    string `help:"Local directory to serve." default:"./tmp/uploads"`
	Port        int    `help:"Port to listen on." default:"8082"`
	FormKey     string `help:"File upload request form key name." default:"files"`
	MaxFileSize int64  `help:"Maximum file size in megabytes." default:"50"`
}

type MockServerOptions struct {
	Port     int    `help:"Port to listen on." default:"8081"`
	Size     int    `help:"Number of records to generate." default:"100"`
	CsvFiles string `help:"CSV files to read as data, separated by semi-colon" default:""`
}

type OAuthServerOptions struct {
	Port int `help:"Port to listen on." default:"8083"`
}

type Options struct {
	FileServer  FileServerOptions  `cmd:"" name:"file-server" help:"Start a mock file server to receive files."`
	MockServer  MockServerOptions  `cmd:"" name:"mock-server" help:"Start a mock server to receive requests."`
	OAuthServer OAuthServerOptions `cmd:"" name:"oauth-server" help:"Start a mock oauth server to receive requests."`
}
