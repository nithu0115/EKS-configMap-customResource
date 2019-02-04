### EKS-configMap-customResource

EKS-configMap-customResource is a AWS Lambda backed custom resource to create or update config map with Node Instance Role which would automatically allow worker nodes to join the cluster. 

#### Pre-Requisites

1) git clone to check out the repository to local and cd to the directory
2) Edit Makefile and update **S3Bucket** variable:
```S3Bucket ?= eks-lambda-configmap-customresource```

3) make all (_Note: You will need to install dep package: https://golang.github.io/dep/docs/installation.html before running make all command_) 


```
make all
Checking dependencies...
Building...
Zip binary...
updating: main (deflated 70%)
Sam packing...
Running aws cloudformation package --template-file sam.yaml --s3-bucket eks-lambda-configmap-customresource --output-template-file sam-packaged.yaml
Uploading to 9ab49ec8a813c54f7720685f8175fb67  8075063 / 8075063.0  (100.00%)
Successfully packaged artifacts and wrote output template to file sam-packaged.yaml.
Execute the following command to deploy the packaged template
aws cloudformation deploy --template-file /Users/nithmu/go/src/github.com/aws/eks_saver/sam-packaged.yaml --stack-name <YOUR STACK NAME>
Sam deploying CFN stack/changeSets ...

Waiting for changeset to be created..
Waiting for stack create/update to complete
Successfully created/updated stack - eks-lambda-configMap-customResource
```


