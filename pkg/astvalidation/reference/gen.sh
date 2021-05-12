#!/bin/bash

cd testsgo
rm -f *_test.go
cd ..

go run main.go
gofmt -w testsgo
