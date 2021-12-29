package main

import (
	"SuperQueueRequestRouter/logger"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/http2"
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
	client http.Client
)

func StartHTTPServer() {
	echoInstance := echo.New()
	Server = &HTTPServer{
		Echo: echoInstance,
	}
	Server.Echo.HideBanner = true
	Server.Echo.Use(MetricsHandler)
	// Server.Echo.Use(middleware.Logger())
	config := middleware.LoggerConfig{
		Skipper: middleware.DefaultSkipper,
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
			`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}",` +
			`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}"` +
			`,"bytes_in":${bytes_in},"bytes_out":${bytes_out},"proto":"${protocol}"}` + "\n",
		CustomTimeFormat: "2006-01-02 15:04:05.00000",
	}
	Server.Echo.Use(middleware.LoggerWithConfig(config))
	Server.Echo.Validator = &CustomValidator{validator: validator.New()}
	t := &http2.Transport{
		AllowHTTP: true,
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	// t.ConnPool
	// t.MaxIdleConns = 500
	// t.MaxConnsPerHost = 500
	// t.MaxIdleConnsPerHost = 500
	client = http.Client{
		Timeout:   10 * time.Second,
		Transport: t,
	}

	// Count requests
	Server.registerRoutes()

	logger.Info("Starting SuperQueueRequestRouter on port ", GetEnvOrDefault("HTTP_PORT", "9090"))
	s := http.Server{
		Handler:      Server.Echo,
		Addr:         ":" + GetEnvOrDefault("HTTP_PORT", "9090"),
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)), // force disable http2
	}
	if err := s.ListenAndServeTLS("domain.crt", "domain.key"); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (s *HTTPServer) registerRoutes() {
	s.Echo.GET("/hc", func(c echo.Context) error {
		return c.String(200, "y")
	})

	s.Echo.POST("/record", Post_Record)
	s.Echo.GET("/record", Get_Record)

	s.Echo.POST("/ack/:recordID", Post_AckRecord)
	s.Echo.POST("/nack/:recordID", Post_NackRecord)

	s.Echo.GET("/metrics", wrapPromHandler)
	SetupMetrics()
}

func MetricsHandler(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		next(c) // Wait for all other handlers
		TotalRequestsCounter.Inc()
		theUrl := c.Request().URL.String()
		// We don't want the cardinality of record ids destroying our metrics
		if strings.HasPrefix(c.Request().URL.String(), "/ack") {
			theUrl = "/ack"
		} else if strings.HasPrefix(c.Request().URL.String(), "/nack") {
			theUrl = "/nack"
		}
		HTTPResponsesMetric.WithLabelValues(fmt.Sprintf("%d", c.Response().Status), theUrl).Inc()
		HTTPLatenciesMetric.WithLabelValues(fmt.Sprintf("%d", c.Response().Status), theUrl).Observe(float64(time.Since(start) / time.Millisecond))
		return nil
	}
}

func wrapPromHandler(c echo.Context) error {
	h := promhttp.Handler()
	h.ServeHTTP(c.Response(), c.Request())
	return nil
}

func Post_Record(c echo.Context) error {
	req := c.Request()
	reqbody, err := ioutil.ReadAll(c.Request().Body)
	req.Body = ioutil.NopCloser(bytes.NewReader(reqbody))
	defer req.Body.Close()
	body := new(PostRecordRequest)
	if err := ValidateRequest(c, body); err != nil {
		logger.Debug("Validation failed ", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid body")
	}

	if err != nil {
		return c.String(500, "Failed to parse body")
	}

	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}

	// Make post request on requester behalf

	// Get known partitions for a queue
	s := time.Now()
	partitions, err := GetQueuePartitions(context.Background(), queue)
	if err != nil {
		logger.Error("Error getting random partitions for queue ", queue)
		logger.Error(err)
		return c.String(500, "Failed to get partitions for queue")
	}
	if len(*partitions) == 0 {
		return echo.NewHTTPError(404, "Queue not found")
	}
	logger.Debug("Got partitions ", partitions, " in ", time.Since(s))
	s = time.Now()
	randomPartition := GetRandomPartition(partitions)

	newURL := randomPartition.Address + "/record"
	newReq, err := http.NewRequest(req.Method, newURL, bytes.NewReader(reqbody))
	if err != nil {
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

	s = time.Now()
	resp, err := client.Do(newReq)
	if err != nil {
		logger.Error("failed to forward request")
		logger.Error(err)
		return c.String(500, "Failed to forward request")
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Error("parse response body")
		logger.Error(err)
		return c.String(500, "parse response body")
	}

	logger.Debug("Did post in ", time.Since(s))

	return c.Blob(resp.StatusCode, resp.Header.Get("content-type"), respBody)
}

func Get_Record(c echo.Context) error {
	req := c.Request()
	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}

	// Make post request on requester behalf

	// Get known partitions for a queue
	partitions, err := GetQueuePartitions(context.Background(), queue)
	if err != nil {
		logger.Error("Error getting random partitions for queue ", queue)
		logger.Error(err)
		return c.String(500, "Failed to get partitions for queue")
	}
	if len(*partitions) == 0 {
		return echo.NewHTTPError(404, "Queue not found")
	}

	randomPartition := GetRandomPartition(partitions)
	logger.Debug("Got random partition ", randomPartition.Partition, " at ", randomPartition.Address)

	newURL := randomPartition.Address + "/record"
	newReq, err := http.NewRequest(req.Method, newURL, nil)
	if err != nil {
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
		logger.Error("failed to forward request")
		logger.Error(err)
		return c.String(500, "Failed to forward request")
	}

	if resp.StatusCode == 204 {
		return c.String(204, "")
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Error("parse response body")
		logger.Error(err)
		return c.String(500, "parse response body")
	}

	return c.Blob(resp.StatusCode, resp.Header.Get("content-type"), respBody)
}

func Post_AckRecord(c echo.Context) error {
	req := c.Request()

	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}
	recordID := c.Param("recordID")
	if recordID == "" {
		return c.String(400, "No record ID given")
	}

	partitionID := strings.Split(recordID, "_")[0]
	if partitionID == "" {
		return c.String(http.StatusBadRequest, "Bad record ID given")
	}

	// Make post request on requester behalf

	// Get known partitions for a queue
	partitions, err := GetQueuePartitions(context.Background(), queue)
	if err != nil {
		logger.Error("Error getting random partitions for queue ", queue)
		logger.Error(err)
		return c.String(500, "Failed to get partitions for queue")
	}
	if len(*partitions) == 0 {
		return echo.NewHTTPError(404, "Queue not found")
	}
	partition := GetPartition(partitions, partitionID)
	logger.Debug("Got ack partition ", partition.Address)

	newURL := partition.Address + "/ack/" + recordID
	newReq, err := http.NewRequest(req.Method, newURL, nil)
	if err != nil {
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
		logger.Error("failed to forward request")
		logger.Error(err)
		return c.String(500, "Failed to forward request")
	}

	logger.Debug("Got status back for ack ", resp.StatusCode)

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Error("parse response body")
		logger.Error(err)
		return c.String(500, "parse response body")
	}

	return c.Blob(resp.StatusCode, resp.Header.Get("content-type"), respBody)
}

func Post_NackRecord(c echo.Context) error {
	req := c.Request()

	// Get queue header
	queue := c.Request().Header.Get("sq-queue")
	if queue == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid queue header")
	}
	recordID := c.Param("recordID")
	if recordID == "" {
		return c.String(400, "No record ID given")
	}

	partitionID := strings.Split(recordID, "_")[0]
	if partitionID == "" {
		return c.String(http.StatusBadRequest, "Bad record ID given")
	}

	bodyBytes, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		logger.Error("Failed to read body bytes:")
		logger.Error(err)
	}
	c.Request().Body = ioutil.NopCloser(bytes.NewReader(bodyBytes))
	body := new(NackRecordRequest)
	if err := ValidateRequest(c, body); err != nil {
		logger.Debug("Validation failed ", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid body")
	}

	// Make post request on requester behalf

	// Get known partitions for a queue
	partitions, err := GetQueuePartitions(context.Background(), queue)
	if err != nil {
		logger.Error("Error getting random partitions for queue ", queue)
		logger.Error(err)
		return c.String(500, "Failed to get partitions for queue")
	}
	if len(*partitions) == 0 {
		return echo.NewHTTPError(404, "Queue not found")
	}
	partition := GetPartition(partitions, partitionID)
	logger.Debug("Got nack partition ", partition.Address)

	newURL := partition.Address + "/nack/" + recordID
	bodyBuffer := bytes.NewBuffer(bodyBytes)
	newReq, err := http.NewRequest(req.Method, newURL, bodyBuffer)
	if err != nil {
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
		logger.Error("failed to forward request")
		logger.Error(err)
		return c.String(500, "Failed to forward request")
	}

	logger.Debug("Got status back for nack ", resp.StatusCode)

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Error("parse response body")
		logger.Error(err)
		return c.String(500, "parse response body")
	}

	return c.Blob(resp.StatusCode, resp.Header.Get("content-type"), respBody)
}
