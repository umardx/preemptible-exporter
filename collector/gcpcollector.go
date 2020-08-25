package gcpcollector

import (
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/host"
	log "github.com/sirupsen/logrus"
)

type preemptingMetric struct {
	httpCli             *http.Client
	metadataEndpoint    string
	scrapeSuccessful    *prometheus.Desc
	preemptingIndicator *prometheus.Desc
	preemptingUpTime    *prometheus.Desc
}

func NewPreemptionExporter(me string) *preemptingMetric {
	netTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 2 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: netTransport,
	}
	return &preemptingMetric{
		httpCli:             client,
		metadataEndpoint:    me,
		scrapeSuccessful:    prometheus.NewDesc("gcp_instance_metadata_status_available", "Metadata status available", []string{"instance_id", "instance_name"}, nil),
		preemptingIndicator: prometheus.NewDesc("gcp_instance_preempted_indicator", "Instance is about to be preempted", []string{"instance_id", "instance_name", "preempted"}, nil),
		preemptingUpTime:    prometheus.NewDesc("gcp_instance_preempted_uptime", "Instance will be preempted in", []string{"instance_id", "instance_name"}, nil),
	}
}

func (c *preemptingMetric) get(path string) (string, error) {
	req, err := http.NewRequest("GET", c.metadataEndpoint+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", errors.New("endpoint not fount")
	}
	return string(body), nil
}

func (c *preemptingMetric) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.scrapeSuccessful
	ch <- c.preemptingIndicator
	ch <- c.preemptingUpTime
}

func (c *preemptingMetric) Collect(ch chan<- prometheus.Metric) {
	var preemptedValue float64
	log.Info("Fetching termination data from metadata-service")
	instanceID, err := c.get("id")
	if err != nil {
		log.Errorf("couldn't parse instance id from metadata: %s", err.Error())
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessful, prometheus.GaugeValue, 0, "none", "none")
		return
	}
	instanceName, err := c.get("name")
	if err != nil {
		log.Errorf("couldn't parse instance name from metadata: %s", err.Error())
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessful, prometheus.GaugeValue, 0, instanceID, "none")
		return
	}
	preempted, err := c.get("preempted")
	if err != nil {
		log.Errorf("Failed to fetch data from metadata service: %s", err)
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessful, prometheus.GaugeValue, 0, instanceID, instanceName)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.scrapeSuccessful, prometheus.GaugeValue, 1, instanceID, instanceName)
	log.Infof("instance endpoint available, will be preempted: %v", preempted)
	if isPreempted, _ := strconv.ParseBool(preempted); isPreempted {
		preemptedValue = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.preemptingIndicator, prometheus.GaugeValue, preemptedValue, instanceID, instanceName, preempted)
	uptime, _ := host.Uptime()
	log.Infof("instance was started at : %v", uptime)
	ch <- prometheus.MustNewConstMetric(c.preemptingUpTime, prometheus.GaugeValue, float64(uptime), instanceID, instanceName)
}
