package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"time"

	"github.com/antigloss/go/logger"
	"github.com/flavio-fernandes/mqtt2ping/internal/mqtt_agent"
	"github.com/go-ping/ping"
	"github.com/mitchellh/mapstructure"
	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"
)

const (
	defaultIntervalSeconds             = 3
	defaultUpdateStatusIntervalSeconds = 5

	// minUpdateStatusIntervalSeconds is the smallest value we can safely use as status insterval
	minUpdateStatusIntervalSeconds = 2
)

type destinationJson struct {
	Address  string
	Interval int
}

type Destination struct {
	Name                string
	Addr                string `mapstructure:"address"`
	IntervalSeconds     int    `mapstructure:"interval"`
	pinger              *ping.Pinger
	lastPacketsSent     int
	lastPacketsRecv     int
	lastIsOnline        bool
	consecutiveOfflines int
}

type Destinations struct {
	DefaultIntervalSeconds      int           `mapstructure:"interval"`
	AdvertisementsSeconds       int           `mapstructure:"advertisements"`
	UpdateStatusIntervalSeconds int           `mapstructure:"update-interval"`
	Destinations                []Destination `mapstructure:"destinations"`
}

type Manager struct {
	StopChan                    chan struct{}
	defaultIntervalSeconds      int
	advertisementsSeconds       int
	updateStatusIntervalSeconds int
	destinationMap              map[string]*Destination
	mqttPub                     chan<- mqtt_agent.Msg
	mqttSub                     <-chan mqtt_agent.Msg
}

func (m *Manager) parseYaml(configFilename string) error {
	var err error
	var raw interface{}

	if configFilename == "" {
		logger.Warn("No config yaml file provided")
	} else {
		f, err := os.ReadFile(configFilename)
		if err != nil {
			return fmt.Errorf("unable to open pinger destinations %s: %w", configFilename, err)
		}

		// Unmarshal our input YAML file into empty interface
		if err = yaml.Unmarshal(f, &raw); err != nil {
			return fmt.Errorf("unable to parse yaml pinger destinations %s: %w", configFilename, err)
		}
	}

	// Use mapstructure to convert our interface{} to Pinger destinations
	d := Destinations{}
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{WeaklyTypedInput: true, Result: &d})
	if err = decoder.Decode(raw); err != nil {
		return fmt.Errorf("unable to assemble pinger destinations %s: %w", configFilename, err)
	}

	// Assemble m.destinationMap from local copy of Destinations
	if d.DefaultIntervalSeconds != 0 {
		m.defaultIntervalSeconds = d.DefaultIntervalSeconds
	}
	m.advertisementsSeconds = d.AdvertisementsSeconds
	if d.UpdateStatusIntervalSeconds != 0 {
		m.updateStatusIntervalSeconds = d.UpdateStatusIntervalSeconds
	}
	for _, destination := range d.Destinations {
		m.addDestination(destination)
	}

	if len(m.destinationMap) == 0 {
		logger.Warn("No valid pinger destinations from yaml data: use mqtt add topic.")
	}

	return nil
}

func (m *Manager) addDestination(destination Destination) {
	if destination.Addr == "" {
		logger.Warnf("Ignoring destination, due to no address: %#v", destination)
		return
	}

	if destination.Name == "" {
		destination.Name = destination.Addr
	}

	if _, ok := m.destinationMap[destination.Name]; ok {
		logger.Warnf("Ignoring duplicate destination name: %s", destination.Name)
		return
	}

	pinger, err := ping.NewPinger(destination.Addr)
	if err != nil {
		logger.Warnf("Ignoring invalid destination %s: %v", destination.Name, err)
		return
	}
	destination.pinger = pinger

	if destination.IntervalSeconds == 0 {
		destination.IntervalSeconds = m.defaultIntervalSeconds
	}
	destination.pinger.Interval = time.Duration(destination.IntervalSeconds) * time.Second
	destination.pinger.SetPrivileged(true)

	m.destinationMap[destination.Name] = &destination
	logger.Infof("Added destination %s (%s)", destination.Name, pinger.IPAddr().IP.String())

	go func() {
		if err := destination.pinger.Run(); err != nil {
			logger.Errorf("Unable to kick off pinger for destination %s: %w", destination.Name, err)
		}
	}()
}

func (m *Manager) handleUpdateStatusTick() {
	for _, destination := range m.destinationMap {
		stats := destination.pinger.Statistics()
		isOnline := false
		packetsSentSinceLastIter := stats.PacketsSent - destination.lastPacketsSent

		if destination.lastPacketsRecv != stats.PacketsRecv {
			isOnline = true
		} else if packetsSentSinceLastIter <= 1 {
			// may have just sent a packet, so no panic about it, yet
			continue
		}

		isOnlineChanged := isOnline != destination.lastIsOnline
		destination.lastPacketsRecv = stats.PacketsRecv
		destination.lastPacketsSent = stats.PacketsSent
		destination.lastIsOnline = isOnline

		logger.Tracef("%s pinger %s sent: %d received: %d (%.0f%% loss) isOnline: %t changed: %t consecOffline: %d",
			destination.Name, destination.pinger.IPAddr().IP.String(), destination.lastPacketsSent, destination.lastPacketsRecv,
			stats.PacketLoss, destination.lastIsOnline, isOnlineChanged, destination.consecutiveOfflines)

		if isOnline {
			if isOnlineChanged {
				logger.Infof("%s pinger is now online", destination.Name)
				m.publishDestination(destination)
			}
			destination.consecutiveOfflines = 0
		} else {
			// is offline. Notify if this happens 3 times in a row.
			destination.consecutiveOfflines += 1
			if destination.consecutiveOfflines == 3 {
				logger.Infof("%s pinger is now offline", destination.Name)
				m.publishDestination(destination)
			}
		}
	}
}

func (m *Manager) msgParseStatus(topic, payload string) {
	name, ok := mqtt_agent.GetTopicSubDestinationStatus(topic)
	if !ok {
		logger.Errorf("Unexpected parsing of topic:%s", topic)
		return
	}
	if payload != "" {
		logger.Warnf("Ignoring unused payload: %s", payload)
	}
	if name == "" {
		m.publishAllDestinations()
		return
	}
	if destination, ok := m.destinationMap[name]; ok {
		m.publishDestination(destination)
	} else {
		logger.Warnf("No status for destination %s: not-found", name)
	}

}

func (m *Manager) msgParseConfig(topic, payload string) {
	name, ok := mqtt_agent.GetTopicSubDestinationConfig(topic)
	if !ok {
		logger.Errorf("Unexpected parsing of topic: %s", topic)
		return
	}
	if payload != "" {
		m.handleDestinationMsgAdd(name, payload)
	} else {
		m.handleDestinationMsgDel(name, true)
	}
}

func (m *Manager) handleDestinationMsgAdd(name, payload string) {
	// replace existing destination by removing existing one
	m.handleDestinationMsgDel(name, false)

	var destination Destination
	if json.Valid([]byte(payload)) {
		var dest destinationJson
		if err := json.Unmarshal([]byte(payload), &dest); err != nil {
			logger.Errorf("Unexpected json unmarshal error (%s): %w", payload, err)
			return
		}
		destination = Destination{
			Name:            name,
			Addr:            dest.Address,
			IntervalSeconds: dest.Interval,
		}
	} else {
		destination = Destination{
			Name: name,
			Addr: payload,
		}
	}
	m.addDestination(destination)
}

func (m *Manager) handleDestinationMsgDel(name string, logNotFound bool) {
	destination, ok := m.destinationMap[name]
	if !ok {
		if logNotFound {
			logger.Warnf("Ignoring removal of destination name %s: not-found", name)
		}
		return
	}

	destination.pinger.Stop()
	delete(m.destinationMap, name)
	logger.Infof("Removed destination %s (%s)", name, destination.pinger.IPAddr().IP.String())
}

func (m *Manager) publishDestination(destination *Destination) {
	msg := &mqtt_agent.Msg{}
	msg.Topic, msg.Payload = mqtt_agent.MsgPubAdvState(destination.Name, destination.lastIsOnline)
	m.mqttPub <- *msg

	destinationState := msg.Payload
	destinationIP := destination.pinger.IPAddr().String()

	// https://github.com/tidwall/sjson
	stats := destination.pinger.Statistics()
	values := map[string]string{
		"name":                 destination.Name,
		"address":              destination.Addr,
		"ip":                   destinationIP,
		"packets_sent":         fmt.Sprintf("%d", destination.lastPacketsSent),
		"packets_received":     fmt.Sprintf("%d", destination.lastPacketsRecv),
		"interval_in_seconds":  fmt.Sprintf("%d", destination.IntervalSeconds),
		"is_online":            fmt.Sprintf("%t", destination.lastIsOnline),
		"consecutive_offline":  fmt.Sprintf("%d", destination.consecutiveOfflines),
		"rtt_in_milliseconds":  fmt.Sprintf("%v", stats.AvgRtt.Milliseconds()),
		"packets_loss_percent": fmt.Sprintf("%.0f%%", stats.PacketLoss),
	}

	// Do it once and keep it forever
	if gDestinationAttrs == nil {
		gDestinationAttrs = make([]string, 0, len(values))
		for k := range values {
			gDestinationAttrs = append(gDestinationAttrs, k)
		}
		sort.Strings(gDestinationAttrs)
		logger.Infof("Assembled publish info keys cache: %v", gDestinationAttrs)
	}

	jsonValue := ""
	for _, k := range gDestinationAttrs {
		jsonValue, _ = sjson.Set(jsonValue, k, values[k])
	}
	msg.Topic, msg.Payload = mqtt_agent.MsgPubAdvInfo(destination.Name, jsonValue)
	m.mqttPub <- *msg

	logger.Infof("Published destination %s (%s): %s", destination.Name, destinationIP, destinationState)
}

func (m *Manager) publishAllDestinations() {
	logger.Infof("Publising %d destinations", len(m.destinationMap))
	for _, destination := range m.destinationMap {
		m.publishDestination(destination)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *Manager) mainLoop() {
	defer func() { close(m.StopChan) }()
	timeout := time.After(1 * time.Hour)
	var advertiseTick *time.Ticker
	if m.advertisementsSeconds > 0 {
		advertiseTick = time.NewTicker(time.Duration(m.advertisementsSeconds) * time.Second)
		logger.Infof("Advertisements will be sent every: %v seconds", m.advertisementsSeconds)
	} else {
		advertiseTick = time.NewTicker(time.Duration(1<<63 - 1)) // approximately 290 years
	}
	updateStatusInterval := max(minUpdateStatusIntervalSeconds, m.updateStatusIntervalSeconds)
	updateStatusTick := time.NewTicker(time.Duration(updateStatusInterval) * time.Second)
	logger.Infof("Checking for pinger updates every: %v seconds", updateStatusInterval)

	topic, _ := mqtt_agent.MsgPubAdvState("#", true)
	logger.Infof("For destination status, mqtt subscribe to topic: %s", topic)
	topic, _ = mqtt_agent.MsgPubAdvInfo("#", "")
	logger.Infof("For destination details, mqtt subscribe to topic: %s", topic)

	// Listen for Ctrl-C.
	osSignalChn := make(chan os.Signal, 1)
	signal.Notify(osSignalChn, os.Interrupt)

	var msg mqtt_agent.Msg
mgrloop:
	for {
		select {
		case msg = <-m.mqttSub:
			switch msg.Topic {
			case mqtt_agent.GetTopicSubStatus(msg.Topic):
				m.msgParseStatus(msg.Topic, msg.Payload)
			case mqtt_agent.GetTopicSubConfig(msg.Topic):
				m.msgParseConfig(msg.Topic, msg.Payload)
			default:
				logger.Infof("Unhandled: topic %s payload %q...", msg.Topic, mqtt_agent.FirstN(msg.Payload, 10))
			}
		case <-osSignalChn:
			break mgrloop
		case <-advertiseTick.C:
			m.publishAllDestinations()
		case <-updateStatusTick.C:
			m.handleUpdateStatusTick()
		case <-timeout:
			logger.Info("manager happy loop")
		}
	}

	// closing time
	for _, destination := range m.destinationMap {
		logger.Tracef("stopping pinger %s", destination.Name)
		destination.pinger.Stop()
	}
	logger.Info("manager main loop is finished")
}

func Start(mqttPub chan<- mqtt_agent.Msg, mqttSub <-chan mqtt_agent.Msg, config string) (*Manager, error) {
	mgr := Manager{
		StopChan:                    make(chan struct{}),
		defaultIntervalSeconds:      defaultIntervalSeconds,
		updateStatusIntervalSeconds: defaultUpdateStatusIntervalSeconds,
		destinationMap:              make(map[string]*Destination),
		mqttPub:                     mqttPub,
		mqttSub:                     mqttSub,
	}

	if err := mgr.parseYaml(config); err != nil {
		return nil, err
	}

	go mgr.mainLoop()
	return &mgr, nil
}

var gDestinationAttrs []string
