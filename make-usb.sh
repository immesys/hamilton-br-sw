#!/bin/bash

function usage {
  echo "usage: mkusb <device>"
  echo "e.g mkusb /dev/sda"
  echo "DO NOT MAKE A MISTAKE!"
}

if [ -z "$1" ]
then
  usage
  exit 1
fi

DISK=$1

sgdisk -Z $DISK
sgdisk -o $DISK
sgdisk --new=1:0:+16G $DISK
sgdisk --new=2:0:0 $DISK
partprobe
mkfs.vfat ${DISK}1
mkfs.ext4 ${DISK}2

mkdir -p p1
mkdir -p p2
mount ${DISK}1 p1
mount ${DISK}2 p2

mkdir -p p1/interfaces.d

cat <<EOM >p1/interfaces.d/interfaces
# interfaces(5) file used by ifup(8) and ifdown(8)

# The main network device
auto eth0
allow-hotplug eth0
iface eth0 inet dhcp

# Example static
# iface eth0 inet static
# address 10.4.10.240
# netmask 255.255.255.0
# gateway 10.4.10.1
# dns-nameservers 8.8.8.8 8.8.4.4
EOM

cat <<EOM >p1/README.md
# Hamilton BR configuration

Read config.ini, modify it.
EOM

cat <<EOM >p1/config.ini
# the identity of the BR. Keep this unique if you want
# to avoid bad things
POP_ID=mypopID
# the URI to associate the L7G with. This would become:
# my/url/$POP_ID/s.hamilton/<mac of sensor>/i.l7g/signal/+
POP_BASE_URI=my/url
# /config references the first partition of the flash drive
BW2_DEFAULT_ENTITY=/config/an_entity.ent
EOM
