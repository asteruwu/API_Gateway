package main

import (
	"API_Gateway/backend"
	"API_Gateway/builder"
	"API_Gateway/connector"
	"API_Gateway/handler"
	"fmt"
)

func main() {
	// 构建配置
	table := builder.Build()
	// 初始化
	bk := backend.NewBManager(table.Backend)
	hdl := handler.NewHandler(table.Handler, bk.Call)
	lst := connector.NewListener(table.Connector, hdl.HandleHTTPConn)
	// 主流程启动
	err := lst.Connect()
	if err != nil {
		fmt.Printf("[main]failed to start service, err message: %s", err)
	}
}
