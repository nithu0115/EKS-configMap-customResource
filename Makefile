HANDLER ?= main
PACKAGE ?= $(HANDLER)
GOPATH  ?= $(HOME)/go
GOOS    ?= linux
GOOSDEV	?= darwin
GOARCH  ?= amd64

# Modify S3 bucket
S3TMPBUCKET	?= eks-lambda-configMap-customResource
STACKNAME	?= eks-lambda-configMap-customResource

WORKDIR = $(CURDIR:$(GOPATH)%=/go%)
ifeq ($(WORKDIR),$(CURDIR))
	WORKDIR = /tmp
endif

all: dep build zip pack deploy

dep:
	@echo "Checking dependencies..."
	@dep ensure

build:
	@echo "Building..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags='-w -s' -o $(HANDLER)

devbuild:
	@echo "Dev Building..."
	@GOOS=$(GOOSDEV) GOARCH=$(GOARCH) go build -ldflags='-w -s' -o $(HANDLER)

zip:
	@echo "Zip binary..."
	@zip $(PACKAGE).zip $(HANDLER)

clean:
	@echo "Cleaning up..."
	@rm -rf $(HANDLER) $(PACKAGE).zip

pack:
	@echo "Sam packing..."
	@echo "Running aws cloudformation package --template-file sam.yaml --s3-bucket $(S3TMPBUCKET) --output-template-file sam-packaged.yaml"
	@aws cloudformation package --template-file sam.yaml --s3-bucket $(S3TMPBUCKET) --output-template-file sam-packaged.yaml

deploy:
	@echo "Sam deploying CFN stack/changeSets ..."
	@aws cloudformation deploy --template-file sam-packaged.yaml --stack-name $(STACKNAME) --capabilities CAPABILITY_IAM

.PHONY: all dep build zip clean
