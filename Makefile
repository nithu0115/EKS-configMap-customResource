SHELL := /bin/bash # Use bash syntax
HANDLER ?= main
PACKAGE ?= $(HANDLER)
GOPATH  ?= $(HOME)/go
GOOS    ?= linux
GOOSDEV	?= darwin
GOARCH  ?= amd64

# Modify S3 bucket
S3Bucket        ?= eks-lambda-configmap-customresource
STACKNAME	?= eks-lambda-configMap-customResource
#TODO
#RoleARN         ?= @aws iam  

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
	@echo "Sam packing --Running aws cloudformation package --template-file sam.yaml --s3-bucket $(S3Bucket) --output-template-file sam-packaged.yaml"
	@aws cloudformation package --template-file sam.yaml --s3-bucket $(S3Bucket) --output-template-file sam-packaged.yaml

deploy-CustomResource-Node-ConfigMap:
	@echo "Deploying CFN Lambda CustomResource for NodeRole k8-configMap stack..."
	@aws cloudformation deploy --template-file sam-packaged.yaml --stack-name $(STACKNAME) --capabilities CAPABILITY_IAM

deploy-EKS-Cluster-control-plane:
	@echo "Deploying EKS control Plane..."
	@echo "Running aws cloudformation deploy --template-file "
        
deploy-EKS-Worker-Nodes:
	@echo "Deploying EKS Worker Nodes"
	@aws cloudformation deploy --template-file template-eks.yaml --stack-name $(STACKNAME) --capabilities CAPABILITY_IAM

.PHONY: all dep build zip clean
