package main

import (
	"SuperQueueRequestRouter/logger"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type HTTPServer struct {
	Echo *echo.Echo
}

var (
	Server *HTTPServer
)

func StartHTTPServer() {
	echoInstance := echo.New()
	Server = &HTTPServer{
		Echo: echoInstance,
	}
	Server.Echo.HideBanner = true
	Server.Echo.Use(middleware.Logger())

	// Count requests
	Server.registerRoutes()

	logger.Info("Starting SuperQueueRequestRouter on port 9090")
	Server.Echo.Logger.Fatal(Server.Echo.Start(":9090"))
}

func (s *HTTPServer) registerRoutes() {
	s.Echo.GET("/hc", func(c echo.Context) error {
		return c.String(200, "y")
	})

	s.Echo.POST("/record", Post_Record, PostRecordLatencyCounter)
	s.Echo.GET("/record", Get_Record, GetRecordLatencyCounter)

	s.Echo.POST("/ack/:recordID", Post_AckRecord, AckRecordLatencyCounter)
	s.Echo.POST("/nack/:recordID", Post_NackRecord, NackRecordLatencyCounter)
}
