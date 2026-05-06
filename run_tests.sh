#!/bin/bash
go test ./... > /tmp/test_output.txt 2>&1
cat /tmp/test_output.txt
