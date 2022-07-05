#!/usr/bin/env bash

TYK_REPLACE_PATH="github.com\/TykTechnologies\/graphql\-go\-tools"
JENSNEUSE_REPLACE_PATH="github.com\/jensneuse\/graphql\-go\-tools"
WG_REPLACE_PATH="github.com\/wundergraph\/graphql\-go\-tools"

FROM=""
TO=""

function assign_from_path() {
  if [[ $1 == "tyk" ]]; then
    FROM=$TYK_REPLACE_PATH
  elif [[ $1 == "jensneuse" ]]; then
    FROM=$JENSNEUSE_REPLACE_PATH
  else
    FROM=$WG_REPLACE_PATH
  fi
}

function assign_to_path() {
  if [[ $1 == "tyk" ]]; then
      TO=$TYK_REPLACE_PATH
    elif [[ $1 == "jensneuse" ]]; then
      TO=$JENSNEUSE_REPLACE_PATH
    else
      TO=$WG_REPLACE_PATH
    fi
}

if [ -z "$*" ]; then
  echo "usage: reimport.sh -from from -to to"
  exit 1
fi

while test $# -gt 0; do
   case "$1" in
      -from)
        shift
        assign_from_path $1
        shift
        ;;
      -to)
        shift
        assign_to_path $1
        shift
        ;;
      *)
       echo "$1 is not a recognized flag!"
       return 1;
       ;;
  esac
done

IMPORT_PATTERN="s/$FROM/$TO/g"

echo "Replacing ..."
echo "FROM: $FROM"
echo "TO: $TO"
echo "IMPORT_PATTERN: $IMPORT_PATTERN"

if [[ "$OSTYPE" == "darwin"* ]]; then
  find . -type f -iname '*.go' -exec sed -i '' -e $IMPORT_PATTERN '{}' ';'
  find . -type f -iname 'go.mod' -exec sed -i '' -e $IMPORT_PATTERN '{}' ';'
  find . -type f -iname '*gqlgen.yml' -exec sed -i '' -e $IMPORT_PATTERN '{}' ';'
  find . -type f -iname '*.golden' -exec sed -i '' -e $IMPORT_PATTERN '{}' ';'
else
  find . -type f -iname '*.go' -exec sed -i $IMPORT_PATTERN '{}' ';'
  find . -type f -iname 'go.mod' -exec sed -i $IMPORT_PATTERN '{}' ';'
  find . -type f -iname '*gqlgen.yml' -exec sed -i $IMPORT_PATTERN '{}' ';'
  find . -type f -iname '*.golden' -exec sed -i $IMPORT_PATTERN '{}' ';'
fi


printf "\n DONE!\n"

ROOT_DIR=$(pwd)
echo "Tidy examples/chat ..."
cd ./examples/chat
go mod tidy
go generate ./...

echo "Tidy examples/federation and go generate ..."
cd $ROOT_DIR
cd ./examples/federation
go mod tidy
go generate ./...

echo "Tidy examples/kafka_pubsub ..."
cd $ROOT_DIR
cd ./examples/kafka_pubsub
go mod tidy

echo "Tidy ./ ..."
cd $ROOT_DIR
go mod tidy

echo "Go generate in pkg/testing/federationtesting ..."
cd ./pkg/testing/federationtesting
go generate ./...
cd $ROOT_DIR

echo "Go generate in pkg/testing/subscriptiontesting ..."
cd ./pkg/testing/subscriptiontesting
go generate ./...
cd $ROOT_DIR
