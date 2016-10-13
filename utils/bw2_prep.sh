#!/bin/bash

mkdir -p /merged/router
cd /merged/router

if [ ! -e /merged/l3stateok ]
then
  #try download the latest state file to speed up fast sync
  touch /merged/l3stateok
  #this file will be removed if we NAP
fi

if [ ! -e /merged/router/router.ent ]
then
  set -x
  /firmware/bw2 makeconf --logfile /merged/router/log.txt --dbpath /merged/db --conf /merged/router/bw2.ini
fi
