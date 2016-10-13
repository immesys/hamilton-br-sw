#!/bin/bash

if [ -e "/config/network.cfg" ]
then
  . /config/network.cfg
else
  NETWORK=DHCP
  POP_HOSTNAME=hamilton-br
fi

hostname $POP_HOSTNAME

if [ "$NETWORK" = "DHCP" ]
then
  dhclient eth0
elif [ "$NETWORK" = "STATIC" ]
then
  ip addr add $STATIC_IP dev eth0
  ip link set up eth0
  ip route add default via $GATEWAY dev eth0
else
  echo "bad $$NETWORK, using dhcp"
  dhclient eth0
fi

systemctl stop ntp
ntpd -gq
systemctl start ntp
