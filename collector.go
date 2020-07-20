package main

import (
	"encoding/xml"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
)

type zpSupportInfo struct {
	XMLName xml.Name `xml:"ZPSupportInfo"`
	Text    string   `xml:",chardata"`
	File    struct {
		Text string `xml:",chardata"`
		Name string `xml:"name,attr"`
	} `xml:"File"`
}
type sonosdata struct {
	ctl0 int
	ctl1 int
	ctl2 int
	ani  int
}

type device struct {
	DeviceType      string `xml:"deviceType"`
	RoomName        string `xml:"roomName"`
	DisplayVersion  string `xml:"displayVersion"`
	HardwareVersion string `xml:"hardwareVersion"`
	ModelName       string `xml:"modelName"`
	ModelNumber     string `xml:"modelNumber"`
	SerialNum       string `xml:"serialNum"`
	SoftwareVersion string `xml:"softwareVersion"`
	UDN             string `xml:"UDN"`
}

type sonosUnit struct {
	host     string
	roomname string
}

//Define the metrics we wish to expose
var collectionDuration = prometheus.NewDesc("sonos_collection_duration", "Total collection time", nil, nil)
var collectionErrors = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "sonos_collection_errors", Help: "Errors in data collection"}, []string{"host"})
var noiseMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "sonos_noise", Help: "Noise for Sonos ctl"}, []string{"host", "ctl"})
var aniMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "sonos_ani", Help: "AnI value for Sonos ctl"}, []string{"host"})
var hosts = []string{"10.0.0.87", "10.0.0.11"}

func fetchDevice(u string) (*device, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var root struct {
		Device device `xml:"device"`
	}
	if err = xml.NewDecoder(resp.Body).Decode(&root); err != nil {
		log.Printf("Decode %s: %s", u, err)
	}

	return &root.Device, err
}

func init() {
	sonss := getSonosUnits(hosts)
	mycol := jsbCollector{sonosuniots: sonss}
	//Register metrics with prometheus
	prometheus.MustRegister(noiseMetric)
	prometheus.MustRegister(aniMetric)
	prometheus.MustRegister(mycol)
	prometheus.MustRegister(collectionErrors)
	for _, host := range mycol.sonosuniots {
		test, werr := getSonosData(host.host)
		if werr == nil {
			noiseMetric.WithLabelValues(host.roomname, "0").Set(float64(test.ctl0))
			noiseMetric.WithLabelValues(host.roomname, "1").Set(float64(test.ctl1))
			noiseMetric.WithLabelValues(host.roomname, "2").Set(float64(test.ctl2))
			aniMetric.WithLabelValues(host.roomname).Set(float64(test.ani))
			collectionErrors.WithLabelValues(host.roomname).Set(0)
		} else {
			collectionErrors.WithLabelValues(host.roomname).Set(1)
		}

	}
}

func getSonosUnits(sonoshosts []string) []sonosUnit {
	var mysonosUnits = []sonosUnit{}

	for _, host := range sonoshosts {
		dataurl := "http://" + host + ":1400/xml/device_description.xml"
		d, _ := fetchDevice(dataurl)
		mysonosUnits = append(mysonosUnits, sonosUnit{host: host, roomname: d.RoomName})
	}
	return mysonosUnits
}

// Collect implements Prometheus.Collector.
type jsbCollector struct {
	sonosuniots []sonosUnit
}

// Describe implements Prometheus.Collector.
func (c jsbCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collectionDuration

}

// Collect implements Prometheus.Collector.
func (c jsbCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	for _, host := range c.sonosuniots {
		test, werr := getSonosData(host.host)
		if werr == nil {
			noiseMetric.WithLabelValues(host.roomname, "0").Set(float64(test.ctl0))
			noiseMetric.WithLabelValues(host.roomname, "1").Set(float64(test.ctl1))
			noiseMetric.WithLabelValues(host.roomname, "2").Set(float64(test.ctl2))
			aniMetric.WithLabelValues(host.roomname).Set(float64(test.ani))
		} else {
			collectionErrors.WithLabelValues(host.roomname).Inc()
		}

	}
	ch <- prometheus.MustNewConstMetric(collectionDuration, prometheus.GaugeValue, time.Since(start).Seconds())
}

func getSonosData(host string) (sonosdata, error) {
	var sonos sonosdata
	var ani = regexp.MustCompile(`OFDM ANI level: (?P<ani>\d+)`)
	var noise = regexp.MustCompile(`Noise Floor: (?P<noise>-\d+) dBm \(chain (?P<ctl>\d+) ctl\)`)
	// Make HTTP GET request
	client := http.Client{
		Timeout: time.Duration(5 * time.Second),
	}
	response, err := client.Get("http://" + host + ":1400/status/proc/ath_rincon/status")
	if err != nil {
		log.Info(err)
		return sonos, err
	}
	defer response.Body.Close()

	// Get the response body as a string
	dataInBytes, err := ioutil.ReadAll(response.Body)
	var sonosInfo zpSupportInfo
	xml.Unmarshal(dataInBytes, &sonosInfo)
	var aniMatches = ani.FindStringSubmatch(sonosInfo.File.Text)
	for i, aniMatch := range aniMatches {
		if ani.SubexpNames()[i] != "" {
			iani, _ := strconv.Atoi(aniMatch)
			sonos.ani = iani
		}
	}
	var ctlindex int
	var noiseindex int
	for index, value := range noise.SubexpNames() {
		switch value {
		case "ctl":
			ctlindex = index
		case "noise":
			noiseindex = index
		}
	}
	var noiseMatches = noise.FindAllStringSubmatch(sonosInfo.File.Text, -1)
	for _, ctls := range noiseMatches {
		ctlid, _ := strconv.Atoi(ctls[ctlindex])
		ino, _ := strconv.Atoi(ctls[noiseindex])
		switch ctlid {
		case 0:
			sonos.ctl0 = ino
		case 1:
			sonos.ctl1 = ino
		case 2:
			sonos.ctl2 = ino
		}
	}
	return sonos, nil
}
