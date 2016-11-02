#!/bin/bash

if [ -e "/config/network.cfg" ]
then
  . /config/network.cfg
else
  NETWORK=DHCP
  POP_HOSTNAME=hamilton-br
fi

hostname $POP_HOSTNAME

if [ ! -z "$SET_MAC" ]
then
  ip link set eth0 address $SET_MAC
fi
if [ "$NETWORK" = "DHCP" ]
then
  dhclient eth0
elif [ "$NETWORK" = "STATIC" ]
then
  ip addr add $STATIC_IP dev eth0
  ip link set up eth0
  ip route add default via $GATEWAY dev eth0
fi

systemctl stop ntp
ntpd -gq
systemctl start ntp
