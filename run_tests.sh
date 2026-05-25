#!/bin/bash
go test ./... -count=1  > _tmp/test_output.txt 2>&1
cat _tmp/test_output.txt
