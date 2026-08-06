package main

import (
	"API_Gateway/backend"
	"API_Gateway/builder"
	"API_Gateway/connector"
	"API_Gateway/handler"
)

func main() {
	// 构建配置
	table := builder.Build()
	// 初始化
	backend := backend.NewBManager(table.Backend)
	handler := handler.NewHandler(table.Handler, backend.Call)
	listener := connector.NewListener(table.Connector, handler.HandleHTTPConn)
	// 主流程启动
	listener.Connect()
}
