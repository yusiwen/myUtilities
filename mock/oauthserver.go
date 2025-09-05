package mock

import (
	"fmt"
	"log"
	"net/http"

	"github.com/yusiwen/myUtilities/mock/oauth"
)

func (o OAuthServerOptions) Run() error {
	// 创建认证服务器实例
	authServer := oauth.NewAuthServer()

	// 创建HTTP多路复用器
	mux := http.NewServeMux()

	// 设置路由
	authServer.SetupRoutes(mux)

	// 启动服务器
	fmt.Println(fmt.Sprintf("OAuth server started on http://localhost:%d", o.Port))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux))
	return nil
}
