#!/bin/bash

# this script is run at boot. /dev/sda2 is not mounted

umount /merged
umount /volatile
umount /config
umount /firmware

partprobe
if [ ! -e /dev/sda2 ]
then
  echo "Disk not found"
  sleep 300
  reboot
fi

fsck.ext4 -f -p /dev/sda2
if [ $(($? & 188)) -ne 0 ]
then
  echo "/dev/sda2 is pretty corrupt, nuking"
  mkfs.ext4 -F /dev/sda2
  if [ $? -ne 0 ]
  then
    echo "could not format"
    sleep 300
    reboot
  fi
fi

mount -t ext4 /dev/sda2 /volatile
if [ $? -ne 0 ]
then
  echo "could not mount /volatile"
  sleep 300
  reboot
fi

mkdir -p /volatile/upper
mkdir -p /volatile/work


mount -t overlay -o \
lowerdir=/vimage,\
upperdir=/volatile/upper,\
workdir=/volatile/work \
none /merged

if [ $? -ne 0 ]
then
  echo "could not make overlay"
  sleep 300
  reboot
fi

mount -o ro /dev/sda1 /config
if [ $? -ne 0 ]
then
  echo "could not mount config"
  sleep 300
  reboot
fi

mkdir -p /volatile/upper/firmware

if [ -e /config/firmware ]
then
  rsync -PHav /config/firmware/ /volatile/upper/firmware/
  if [ $? -ne 0 ]
  then
    echo "could not copy firmware"
    sleep 300
    reboot
  fi
fi

mount -t overlay -o \
lowerdir=/volatile/upper/firmware:/factory_firmware \
overlay /firmware
if [ $? -ne 0 ]
then
  echo "could not overlay firmware"
  sleep 300
  reboot
fi

# use the time on the fs
/opt/fake-hwclock.sh load
