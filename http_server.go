package main

import (
	"SuperQueueRequestRouter/logger"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type HTTPServer struct {
	Echo *echo.Echo
}

type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	if err := cv.validator.Struct(i); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

func ValidateRequest(c echo.Context, s interface{}) error {
	if err := c.Bind(s); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := c.Validate(s); err != nil {
		return err
	}
	return nil
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

func PostRecordLatencyCounter(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		if err := next(c); err != nil {
			c.Error(err)
		}
		atomic.AddInt64(&PostRecordLatency, int64(time.Since(start)))
		return nil
	}
}

func GetRecordLatencyCounter(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		if err := next(c); err != nil {
			c.Error(err)
		}
		atomic.AddInt64(&GetRecordLatency, int64(time.Since(start)))
		return nil
	}
}

func AckRecordLatencyCounter(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		if err := next(c); err != nil {
			c.Error(err)
		}
		atomic.AddInt64(&AckLatency, int64(time.Since(start)))
		return nil
	}
}

func NackRecordLatencyCounter(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		if err := next(c); err != nil {
			c.Error(err)
		}
		atomic.AddInt64(&NackLatency, int64(time.Since(start)))
		return nil
	}
}

func Post_Record(c echo.Context) error {
	defer atomic.AddInt64(&PostRecordRequests, 1)
	body := new(PostRecordRequest)
	if err := ValidateRequest(c, body); err != nil {
		logger.Debug("Validation failed ", err)
		atomic.AddInt64(&HTTP400s, 1)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid body")
	}

	return c.String(http.StatusCreated, "")
}

func Get_Record(c echo.Context) error {
	defer atomic.AddInt64(&GetRecordRequests, 1)

	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		return c.String(500, "Failed to dequeue record")
	}
	// Empty
	if item == nil {
		atomic.AddInt64(&EmptyQueueResponses, 1)
		return c.String(http.StatusNoContent, "Empty")
	}

}

func Post_AckRecord(c echo.Context) error {
	recordID := c.Param("recordID")
	if recordID == "" {
		atomic.AddInt64(&HTTP400s, 1)
		return c.String(400, "No record ID given")
	}

	partitionID := strings.Split(recordID, "_")[0]
	if partitionID == "" {
		return c.String(http.StatusBadRequest, "Bad record ID given")
	}

	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		return c.String(500, "Failed to ack record")
	}
	return c.String(200, "")
}

func Post_NackRecord(c echo.Context) error {
	recordID := c.Param("recordID")
	if recordID == "" {
		atomic.AddInt64(&HTTP400s, 1)
		return c.String(400, "No record ID given")
	}

	partitionID := strings.Split(recordID, "_")[0]
	if partitionID == "" {
		return c.String(http.StatusBadRequest, "Bad record ID given")
	}

	if err != nil {
		logger.Error(err)
		atomic.AddInt64(&HTTP500s, 1)
		return c.String(500, "Failed to ack record")
	}
	return c.String(200, "")
}
