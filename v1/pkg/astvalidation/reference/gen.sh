#!/bin/bash

cd testsgo
rm -f *Rule_test.go
cd ..

go run main.go
gofmt -w testsgo
