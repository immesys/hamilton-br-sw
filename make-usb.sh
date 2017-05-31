#!/bin/bash
set -ex
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
sleep 10
partprobe
sleep 5
mkfs.fat ${DISK}1
mkfs.ext4 -F ${DISK}2

mkdir -p p1
mkdir -p p2
mount ${DISK}1 p1
mount ${DISK}2 p2

cat <<EOM >p1/network.cfg
# configure the network. At this time, only ethernet
# is supported.
# for dhcp
NETWORK=DHCP

# for static
# NETWORK=STATIC
# STATIC_IP=192.168.1.90/24
# GATEWAY=192.168.1.1
EOM

cat <<EOM >p1/README.md
# Hamilton BR configuration

Edit the network configuration in network.cfg. Then
put your license.lic file in this directory
EOM

sync
set +ex
umount p1
umount p2
umount ${DISK}1
umount ${DISK}2
