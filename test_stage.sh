#!/bin/sh

case $1 in
  '1') 
    CODECRAFTERS_CURRENT_STAGE_SLUG='init'
    ;;
  '2') 
    CODECRAFTERS_CURRENT_STAGE_SLUG='stdio'
    ;;
  '3') 
    CODECRAFTERS_CURRENT_STAGE_SLUG='exit_code'
    ;;
  '4') 
    CODECRAFTERS_CURRENT_STAGE_SLUG='fs_isolation'
    ;;
  '5') 
    CODECRAFTERS_CURRENT_STAGE_SLUG='process_isolation'
    ;;
  '6') 
    CODECRAFTERS_CURRENT_STAGE_SLUG='fetch_from_registry'
    ;;
  *)
    echo 'Invalid stage'
    exit
    ;;
esac

cd docker-tester
go build -o ../docker-go/test.out ./cmd/tester

cd ../docker-go
CODECRAFTERS_SUBMISSION_DIR=$(pwd) \
CODECRAFTERS_CURRENT_STAGE_SLUG=${CODECRAFTERS_CURRENT_STAGE_SLUG} \
./test.out
rm ./test.out