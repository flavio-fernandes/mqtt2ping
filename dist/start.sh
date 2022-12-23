#!/bin/bash

set -o xtrace
set -o errexit

cd "$(dirname $0)"

# https://stackoverflow.com/questions/3174883/how-to-remove-last-directory-from-a-path-with-sed
export TOPDIR="${PWD%/*}"
export PROG=${PROG:-${TOPDIR}/cmd/mqtt2ping/mqtt2ping}
export LOGDIR=${LOGDIR:-${TOPDIR}/log}
export CONFIG=${CONFIG:-${TOPDIR}/data/config_ff.yaml}
export BROKERURL=${BROKERURL:-"tcp://192.168.10.238:1883"}

sudo ${PROG} -broker ${BROKERURL} -config ${CONFIG} -logdir ${LOGDIR} -verbose
