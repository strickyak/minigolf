#!/bin/bash
go test ./... > _tmp/test_output.txt 2>&1
cat _tmp/test_output.txt
