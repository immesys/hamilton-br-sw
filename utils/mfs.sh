#!/bin/bash

# this script is run at boot. /dev/sda2 is not mounted

umount /merged
umount /volatile
umount /config

partprobe
if [ ! -e /dev/sda2 ]
then
  echo "Disk not found"
  exit 1
fi

fsck.ext4 -f -p /dev/sda2
if [ $(($? & 188)) -ne 0 ]
then
  echo "/dev/sda2 is pretty corrupt, nuking"
  mkfs.ext4 -F /dev/sda2
  if [ $? -ne 0 ]
  then
    echo "could not format"
    exit 1
  fi
fi

mount -t ext4 /dev/sda2 /volatile
if [ $? -ne 0 ]
then
  echo "could not mount /volatile"
  exit 1
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
  exit 1
fi

mount -o ro /dev/sda1 /config
if [ $? -ne 0 ]
then
  echo "could not mount config"
  exit 1
fi
