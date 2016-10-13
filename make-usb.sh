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
mkfs.vfat ${DISK}1
mkfs.ext4 -F ${DISK}2

mkdir -p p1
mkdir -p p2
mount ${DISK}1 p1
mount ${DISK}2 p2

cat <<EOM >p1/network.cfg
# configure the hostname
POP_HOSTNAME=hamilton-br

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
edit the BR config in config.ini
EOM

cat <<EOM >p1/config.ini
# the identity of the BR. Keep this unique if you want
# to avoid bad things
POP_ID=mypopID
# the URI to associate the L7G with. This would become:
# my/url/$$POP_ID/s.hamilton/<mac of sensor>/i.l7g/signal/+
POP_BASE_URI=my/url
# /config references the first partition of the flash drive
BW2_DEFAULT_ENTITY=/config/an_entity.ent
EOM

cat <<EOM >p1/authorized_keys
# Place SSH keys that will authenticate for the
# user 'ubuntu' here. No password auth is permitted
#e.g
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC/WfhfBxRdt+i4daMPj2VV0E0W3o9DTOAL7cEP0owd/0PEAv0kAx/AoFGM1aZ/R3w8UNSFmz4eIz2KpSdVrxyf+GyaEpxKUVE6yTwywCU3LT0+9aa/qif6KJGtapnVoSzYphrQ2N2FHPrYtKTT9hf4nEafEKXiSpfOcpOqbhxetsX6WIUIhW+U2YcOwtZ1Hhr5NR3aDbSyUv1uBtBRxTMabBv+M+2mzsPMEwATubC3hrCEbzOG6r6OLBz/anVfCniayBxDNBuzLFT7bLZAZYzgEZWHgU20x8ssJfOI1rdJJrkz9tktcJwJYuzneChXya003eGtDT6RicFfO0eGlUy9 immesys@bunker
EOM

sync
set +ex
umount p1
umount p2
umount ${DISK}1
umount ${DISK}2
