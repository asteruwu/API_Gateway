package main

import (
	"API_Gateway/backend"
	"API_Gateway/builder"
	"API_Gateway/connector"
	"API_Gateway/handler"
	"log"
)

func main() {
	// 构建配置
	table := builder.Build()
	// 初始化
	bk := backend.NewBManager(table.Backend)
	hdl := handler.NewHandler(table.Handler, bk.Call)
	lst, err := connector.NewListener(table.Connector, hdl.HandleHTTPConn)
	if err != nil {
		log.Printf("[main]failed to new listener, errmsg: %s", err.Error())
		return
	}
	// 主流程启动
	err = lst.Connect()
	if err != nil {
		log.Printf("[main]failed to start service, errmsg: %s", err.Error())
	}
}
