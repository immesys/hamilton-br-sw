#!/bin/bash

# we assume mfs has run here already
eval $(/firmware/verifylic /config/license.lic)

if [ "$LIC_VALID" != "true" ]
then
  echo "Invalid license"
  sleep 600
  reboot
fi

ip link set eth0 address $MAC
hostname $KIT_ID

if [ -e "/config/network.cfg" ]
then
  NETWORK=$(cat /config/network.cfg | grep -v "^#" | sed -rn 's/NETWORK=(\w*)\s*$/\1/p')
  if [ "$NETWORK" == "STATIC" ]
  then
    STATICIP=$(cat /config/network.cfg | grep -v "^#" | sed -rn 's/STATIC_IP=([^\s]*)\s*$/\1/p')
    GATEWAY=$(cat /config/network.cfg | grep -v "^#" | sed -rn 's/GATEWAY=([^\s]*)\s*$/\1/p')
  fi
else
  NETWORK=DHCP
fi

if [ "$NETWORK" = "DHCP" ]
then
  dhclient -timeout 1800 eth0
  if [ $? -eq 0 ]
  then
    echo "Successfully completed DHCP"
  else
    echo "DHCP failed"
    reboot
  fi
fi

if [ "$NETWORK" = "STATIC" ]
then
  ip addr add $STATICIP dev eth0
  ip link set up eth0
  ip route add default via $GATEWAY dev eth0
fi

systemctl stop ntp
ntpd -gq
systemctl start ntp
