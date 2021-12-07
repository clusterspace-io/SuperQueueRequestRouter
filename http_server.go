package main

import (
	"SuperQueueRequestRouter/logger"
	"bytes"
	"encoding/json"
	"io/ioutil"
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
	Server.Echo.Validator = &CustomValidator{validator: validator.New()}

	// Count requests
	Server.registerRoutes()

	logger.Info("Starting SuperQueueRequestRouter on port ", GetEnvOrDefault("HTTP_PORT", "9090"))
	Server.Echo.Logger.Fatal(Server.Echo.Start(":" + GetEnvOrDefault("HTTP_PORT", "9090")))
}

func (s *HTTPServer) registerRoutes() {
	s.Echo.GET("/hc", func(c echo.Context) error {
		return c.String(200, "y")
	})

	s.Echo.GET("/partitions", Get_Partitions)
	s.Echo.POST("/record", Post_Record, PostRecordLatencyCounter)
	s.Echo.GET("/record", Get_Record, GetRecordLatencyCounter)

	s.Echo.POST("/ack/:recordID", Post_AckRecord, AckRecordLatencyCounter)
	// s.Echo.POST("/nack/:recordID", Post_NackRecord, NackRecordLatencyCounter)
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

func Get_Partitions(c echo.Context) error {
	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		atomic.AddInt64(&HTTP400s, 1)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}

	// Get the partitions
	partitions, exists := RR.PartitionMap[queue]
	if exists {
		return c.JSON(200, partitions)
	} else {
		return c.String(404, "Queue not found")
	}
}

func Post_Record(c echo.Context) error {
	req := c.Request()
	reqbody, err := ioutil.ReadAll(c.Request().Body)
	req.Body = ioutil.NopCloser(bytes.NewReader(reqbody))
	defer req.Body.Close()
	defer atomic.AddInt64(&PostRecordRequests, 1)
	body := new(PostRecordRequest)
	if err := ValidateRequest(c, body); err != nil {
		logger.Debug("Validation failed ", err)
		atomic.AddInt64(&HTTP400s, 1)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid body")
	}

	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		return c.String(500, "Failed to parse body")
	}

	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		atomic.AddInt64(&HTTP400s, 1)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}

	// Make post request on requester behalf
	client := http.Client{}
	randomPartition := GetRandomPartition(queue)
	logger.Debug("Got random partition ", randomPartition.Address)
	if randomPartition == nil {
		return echo.NewHTTPError(404, "Queue not found")
	}

	newURL := randomPartition.Address + "/record"
	logger.Info("Sending body ", string(reqbody))
	newReq, err := http.NewRequest(req.Method, newURL, bytes.NewReader(reqbody))
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("failed to assemble forwarding request")
		logger.Error(err)
		return c.String(500, "Failed to assemble forwarding request")
	}

	// Forward headers
	newReq.Header = make(http.Header)
	for h, val := range req.Header {
		// TODO: Filter any headers out that are reserved
		newReq.Header[h] = val
	}

	resp, err := client.Do(newReq)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("failed to forward request")
		logger.Error(err)
		return c.String(500, "Failed to forward request")
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("parse response body")
		logger.Error(err)
		return c.String(500, "parse response body")
	}

	return c.String(resp.StatusCode, string(respBody))
}

func Get_Record(c echo.Context) error {
	defer atomic.AddInt64(&GetRecordRequests, 1)
	req := c.Request()

	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		atomic.AddInt64(&HTTP400s, 1)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}

	// Make post request on requester behalf
	client := http.Client{}
	randomPartition := GetRandomPartition(queue)
	logger.Debug("Got random partition ", randomPartition.Address)
	if randomPartition == nil {
		return echo.NewHTTPError(404, "Queue not found")
	}

	newURL := randomPartition.Address + "/record"
	newReq, err := http.NewRequest(req.Method, newURL, nil)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("failed to assemble forwarding request")
		logger.Error(err)
		return c.String(500, "Failed to assemble forwarding request")
	}

	// Forward headers
	newReq.Header = make(http.Header)
	for h, val := range req.Header {
		// TODO: Filter any headers out that are reserved
		newReq.Header[h] = val
	}

	resp, err := client.Do(newReq)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("failed to forward request")
		logger.Error(err)
		return c.String(500, "Failed to forward request")
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("parse response body")
		logger.Error(err)
		return c.String(500, "parse response body")
	}

	var jsonBody map[string]interface{}
	err = json.Unmarshal(respBody, &jsonBody)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("json parse response body")
		logger.Error(err)
		return c.String(500, "json parse response body")
	}

	return c.JSON(resp.StatusCode, jsonBody)
}

func Post_AckRecord(c echo.Context) error {
	req := c.Request()

	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		atomic.AddInt64(&HTTP400s, 1)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}
	recordID := c.Param("recordID")
	if recordID == "" {
		atomic.AddInt64(&HTTP400s, 1)
		return c.String(400, "No record ID given")
	}

	partitionID := strings.Split(recordID, "_")[0]
	if partitionID == "" {
		return c.String(http.StatusBadRequest, "Bad record ID given")
	}

	// Make post request on requester behalf
	client := http.Client{}
	// Get a specific partition
	partitions, exists := RR.PartitionMap[queue]
	if !exists {
		return echo.NewHTTPError(404, "Queue not found")
	}
	var partition *Partition
	for _, i := range partitions {
		if i.ID == partitionID {
			partition = i
		}
	}
	if partition == nil {
		return c.String(404, "Partition not found")
	}
	logger.Debug("Got random partition ", partition.Address)

	newURL := partition.Address + "/ack/" + recordID
	newReq, err := http.NewRequest(req.Method, newURL, nil)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("failed to assemble forwarding request")
		logger.Error(err)
		return c.String(500, "Failed to assemble forwarding request")
	}

	// Forward headers
	newReq.Header = make(http.Header)
	for h, val := range req.Header {
		// TODO: Filter any headers out that are reserved
		newReq.Header[h] = val
	}

	resp, err := client.Do(newReq)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("failed to forward request")
		logger.Error(err)
		return c.String(500, "Failed to forward request")
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		atomic.AddInt64(&HTTP500s, 1)
		logger.Error("parse response body")
		logger.Error(err)
		return c.String(500, "parse response body")
	}

	return c.JSON(resp.StatusCode, respBody)
}

// func Post_NackRecord(c echo.Context) error {
// 	recordID := c.Param("recordID")
// 	if recordID == "" {
// 		atomic.AddInt64(&HTTP400s, 1)
// 		return c.String(400, "No record ID given")
// 	}

// 	partitionID := strings.Split(recordID, "_")[0]
// 	if partitionID == "" {
// 		return c.String(http.StatusBadRequest, "Bad record ID given")
// 	}

// 	if err != nil {
// 		logger.Error(err)
// 		atomic.AddInt64(&HTTP500s, 1)
// 		return c.String(500, "Failed to ack record")
// 	}
// 	return c.String(200, "")
// }
