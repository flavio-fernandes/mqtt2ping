# mqtt2ping
A Golang-based application that monitors destinations (via ping) and notifies changes (via MQTT)

This application leverages [Paho MQTT](https://github.com/eclipse/paho.mqtt.golang) and [Go-Ping](https://github.com/go-ping/ping),
amongst [other libraries](https://github.com/flavio-fernandes/mqtt2ping/blob/8bdde7c15c75964bdc1aa6d12ecc58bf40e2cf81/go.mod#L9-L14)
to monitor multiple Internet addresses with ease. The monitored destinations can be provided upfront using a YAML file, and/or later during
runtime via a well-known MQTT topic format.

## Prepare MQTT broker (optional)

This project uses an MQTT broker for managing its ping destinations as well as sending notifications. The broker to be used
is provided as a parameter. If that parameter is not given, it will use [hivemq.com](https://github.com/flavio-fernandes/mqtt2ping/blob/8bdde7c15c75964bdc1aa6d12ecc58bf40e2cf81/internal/mqtt_agent/mqtt_agent.go#L27).

## YAML config file (optional)

```yaml
---
# MQTT publish the state of all destinations every 10 minutes.
advertisements: 600

# Ping interval of 60 seconds for destinations that do not specify one.
# Default: 3 seconds
interval: 60

destinations:
  # Lookup address
  # Use address as the name
  # Use implicit interval from above (i.e. 60s)
  - address: "localhost"

  - address: "127.0.0.1"
    name: "localhost2"
    interval: 2

  - address: "127.0.0.1"
    name: "localhost3"
    interval: 3

  - address: "9.9.9.9"
    name: "quad9"
    interval: 3600

  - address: "google.com"
    name: "goggle"
    interval: 600
```


## Deployment

- Clone this repo
```bash
git clone https://github.com/flavio-fernandes/mqtt2ping.git && cd mqtt2ping
```

- [Optional] Configure YAML config

See the comments above and [YAML files in this folder](https://github.com/flavio-fernandes/mqtt2ping/tree/main/data) as examples. It is possible to skip
using this file and add the destinations during run time, as shown below.

- **Option A:** Compile Source

- Install Makefile+Golang

For installing Golang compiler, [see this link](https://github.com/flavio-fernandes/ovscon22kind/blob/main/provision/golang.sh).

```bash
dnf group install "Development Tools" ; # for rpm based systems
apt install build-essential ; # for debian/ubuntu based systems

export PATH="/usr/local/go/bin:$PATH"
make build
./dist/mqtt2ping --help
```

- **Option B:** Using Docker

```bash
make build-docker

export MQTT="broker.hivemq.com"
docker run -e DEBUG=1 -e BROKERURL="tcp://${MQTT}:1883" \
-e CONFIG="/data/config.yaml" -v ${PWD}/data:/data:ro \
--name mqtt2ping --rm mqtt2ping
```

## Using MQTT client

For installing mosquitto, [see this link](https://mosquitto.org/download/). But any [MQTT client](https://iot4beginners.com/top-10-different-mqtt-clients-in-2020/) will do.

```bash
export MQTTPREFIX="mqtt2ping"
export MQTT="broker.hivemq.com"

# To monitor the destinations, try something like this:
mosquitto_sub -F '@Y-@m-@dT@H:@M:@S@z : %q : %t : %p' -h $MQTT -t "${MQTTPREFIX}/#"

# Destinations can be dynamically added/updated using the topic+payloads like:
mosquitto_pub -h $MQTT -t "${MQTTPREFIX}/destination/foo1" -m 1.2.3.4
mosquitto_pub -h $MQTT -t "${MQTTPREFIX}/destination/foo2" -m '{"interval": 10, "address":"fd00:10:244:1::4"}' ; # IPv6 is supported
mosquitto_pub -h $MQTT -t "${MQTTPREFIX}/destination/foo3" -m '{"address":"1.1.1.1"}' -r ; # using -r retain to make destination 'persist' across restarts

# To trigger status (i.e. force an advertisement):
mosquitto_pub -h $MQTT -t "${MQTTPREFIX}/status" -n       ; # all
mosquitto_pub -h $MQTT -t "${MQTTPREFIX}/status/foo1" -n  ; # only foo1

# To delete a destination:
mosquitto_pub -h $MQTT -t "${MQTTPREFIX}/destination/foo2" -n
mosquitto_pub -h $MQTT -t "${MQTTPREFIX}/destination/foo3" -r -n
```
