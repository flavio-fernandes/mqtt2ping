package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/antigloss/go/logger"
	"github.com/flavio-fernandes/mqtt2ping/internal/manager"
	"github.com/flavio-fernandes/mqtt2ping/internal/mqtt_agent"
)

const (
	DefaultLogDir = "/tmp/mqtt2ping_log"
)

func getMqttConfig() (mqttConfig *mqtt_agent.Config) {
	mqttConfig = &mqtt_agent.Config{
		ClientId:    mqtt_agent.DefMqttClientId,
		BrokerUrl:   mqtt_agent.DefBrokerURL,
		User:        mqtt_agent.DefBrokerUser,
		Pass:        mqtt_agent.DefBrokerPass,
		TopicPrefix: mqtt_agent.DefTopicPrefix,
	}

	var paramValue string
	if paramValue = os.Getenv("CLIENTID"); paramValue != "" {
		mqttConfig.ClientId = paramValue
	}
	if paramValue = os.Getenv("BROKERURL"); paramValue != "" {
		mqttConfig.BrokerUrl = paramValue
	}
	if paramValue = os.Getenv("MQTTUSER"); paramValue != "" {
		mqttConfig.User = paramValue
	}
	if paramValue = os.Getenv("MQTTPASS"); paramValue != "" {
		mqttConfig.Pass = paramValue
	}
	if paramValue = os.Getenv("PREFIX"); paramValue != "" {
		mqttConfig.TopicPrefix = paramValue
	}
	return
}

func main() {
	verbose := os.Getenv("DEBUG") == "1"
	defaultLogDir := os.Getenv("LOGDIR")
	if defaultLogDir == "" {
		defaultLogDir = DefaultLogDir
	}
	configFilename := os.Getenv("CONFIG")
	mqttConfig := getMqttConfig()

	verboseParamPtr := flag.Bool("verbose", verbose, "enable trace level logs. Can be enabled by setting env DEBUG=1")
	logDirParamPtr := flag.String("logdir", defaultLogDir, "logs directory. Use env LOGDIR to override")
	configParamPtr := flag.String("config", configFilename, "application config yaml file. Use env CONFIG to override")
	clientIdParamPtr := flag.String("client", mqttConfig.ClientId, "mqtt client id. Use env CLIENTID to override. To auto-generate, use 'random'")
	brokerUrlParamPtr := flag.String("broker", mqttConfig.BrokerUrl, "mqtt broker url. Use env BROKERURL to override")
	userParamPtr := flag.String("user", mqttConfig.User, "mqtt username. Use env MQTTUSER to override")
	passParamPtr := flag.String("pass", mqttConfig.Pass, "mqtt password. Use env MQTTPASS to override")
	topicPrefixParamPtr := flag.String("topic", mqttConfig.TopicPrefix, "mqtt topic pinger prefix. Use env PREFIX to override")
	flag.Parse()

	// https://github.com/antigloss/go/blob/92623f0632a424d1c22e37dc039a05f16abe58d6/logger/logger.go#L85
	loggerConfig := logger.Config{
		LogDir:          *logDirParamPtr,
		LogFileMaxSize:  4,
		LogFileMaxNum:   20,
		LogFileNumToDel: 5,
		LogLevel:        logger.LogLevelInfo,
		LogDest:         logger.LogDestBoth,
		Flag:            logger.ControlFlagLogThrough | logger.ControlFlagLogLineNum | logger.ControlFlagLogDate,
	}
	if *verboseParamPtr {
		loggerConfig.LogLevel = logger.LogLevelTrace
	}
	err := logger.Init(&loggerConfig)
	if err != nil {
		fmt.Fprint(os.Stderr, fmt.Sprint("Logger init failed ", *logDirParamPtr, " ", err, "\n"))
		os.Exit(1)
	}
	logger.Infof("starting main application. config yaml: %s", *configParamPtr)

	mqttConfig.ClientId = *clientIdParamPtr
	mqttConfig.BrokerUrl = *brokerUrlParamPtr
	mqttConfig.User = *userParamPtr
	mqttConfig.Pass = *passParamPtr
	mqttConfig.TopicPrefix = *topicPrefixParamPtr

	// Empty mqttConfig.ClientId to make client auto generate one
	if strings.EqualFold(mqttConfig.ClientId, "random") || mqttConfig.ClientId == "" {
		logger.Info("mqtt client id will be auto generated")
		mqttConfig.ClientId = ""
	}

	mqttSubMsgChannel := make(chan mqtt_agent.Msg, 1024)
	mqttPubMsgChannel := mqtt_agent.Start(mqttConfig, mqttSubMsgChannel)
	mgr, err := manager.Start(mqttPubMsgChannel, mqttSubMsgChannel, *configParamPtr)
	if err != nil {
		fmt.Fprint(os.Stderr, fmt.Sprint("Main init failed: ", err, "\n"))
		os.Exit(2)
	}

	<-mgr.StopChan
	logger.Infof("stopping main application")
}
