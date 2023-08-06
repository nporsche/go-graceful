#!/bin/bash
set -x
FILE=./main.pid

if [[ $1 == "clean" ]]; then
  echo "clean start"
  oldPID=`cat $FILE`
  echo $oldPID
  kill $oldPID
  rm unix.sock
  rm $FILE
  echo "clean done"
  exit 0
fi

if [[ $1 == "stop" ]]; then
  echo "stop start"
  oldPID=`cat $FILE`
  echo $oldPID
  kill $oldPID
  rm $FILE
  echo "stop done"
  exit 0
fi

go build server.go

if test -f "$FILE"; then
  oldPID=`cat $FILE`
  echo $oldPID
  kill -s HUP $oldPID
else
  ./server -net unix -listen ./unix.sock -logtostderr
fi

