package mock

type FileServerOptions struct {
	LocalDir    string `help:"Local directory to serve." default:"./tmp/uploads"`
	Port        int    `help:"Port to listen on." default:"8082"`
	FormKey     string `help:"File upload request form key name." default:"files"`
	MaxFileSize int64  `help:"Maximum file size in megabytes." default:"50"`
}

type Options struct {
	FileServer FileServerOptions `cmd:"" name:"file-server" help:"Start a mock file server to receive files."`
}
