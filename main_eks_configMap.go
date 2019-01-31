/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/kubernetes-sigs/aws-iam-authenticator/pkg/token"
	yaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type cluster struct {
	Name       string
	Endpoint   string
	CA         []byte
	MasterRole string
}

type mapRolesData []map[string]interface{}

const (
	//configMapNamespace is the namespace of the ConfigMap
	configMapNamespace = "kube-system"
	//configMapName is the name of the ConfigMap
	configMapName = "aws-auth"
)

func getClusterInfo(clustername string, masterroleARN string, region string) (*cluster, error) {
	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	svc := eks.New(sess)
	input := &eks.DescribeClusterInput{
		Name: aws.String(clustername),
	}

	result, err := svc.DescribeCluster(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case eks.ErrCodeResourceNotFoundException:
				fmt.Println(eks.ErrCodeResourceNotFoundException, aerr.Error())
			case eks.ErrCodeClientException:
				fmt.Println(eks.ErrCodeClientException, aerr.Error())
			case eks.ErrCodeServerException:
				fmt.Println(eks.ErrCodeServerException, aerr.Error())
			case eks.ErrCodeServiceUnavailableException:
				fmt.Println(eks.ErrCodeServiceUnavailableException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return nil, err
	}
	// fmt.Println(result)

	// The CA data comes base64 encoded string inside a JSON object { "Data": "..." }
	ca, err := base64.StdEncoding.DecodeString(*result.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, err
	}

	return &cluster{
		Name:       *result.Cluster.Name,
		Endpoint:   *result.Cluster.Endpoint,
		CA:         ca,
		MasterRole: masterroleARN,
	}, nil
}

func (c *cluster) AuthToken() (string, error) {

	// Init a new aws-iam-authenticator token generator
	gen, err := token.NewGenerator(false)
	if err != nil {
		return "", err
	}

	// Use the current IAM credentials to obtain a K8s bearer token
	tok, err := gen.GetWithRole(c.Name, c.MasterRole)
	if err != nil {
		return "", err
	}

	return tok.Token, nil
}

// Handler is your Lambda function handler
func Handler(ctx context.Context, event cfn.Event) (physicalResourceID string, data map[string]interface{}, err error) {

	//Print CloudFormation request
	fmt.Println(event)

	// Request variables
	clustername := event.ResourceProperties["ClusterName"].(string)
	if len(clustername) < 1 {
		panic("Unable to grab cluster name from ENVIRONMENT variable")
	}

	masterroleARN := event.ResourceProperties["MasterRoleARN"].(string)
	if len(masterroleARN) < 1 {
		panic("Unable to grab Master Role ARN from ENVIRONMENT variable")
	}

	nodeInstRoleARN := event.ResourceProperties["NodeInstanceRoleARN"].(string)
	if len(nodeInstRoleARN) < 1 {
		panic("Unable to grab Node Instance Role ARN from ENVIRONMENT variable")
	}

	// Get Region info from ENV or lambda env
	region := os.Getenv("region")
	if len(region) < 1 {
		region = os.Getenv("AWS_REGION")
	}

	// Get EKS cluster details
	cluster, err := getClusterInfo(clustername, masterroleARN, region)
	fmt.Printf("Amazon EKS Cluster: %s (%s)\n", cluster.Name, cluster.Endpoint)

	// Use the aws-iam-authenticator to fetch a K8s authentication bearer token
	token, err := cluster.AuthToken()
	if err != nil {
		panic("Failed to obtain token from aws-iam-authenticator, " + err.Error())
	}

	// Create a new K8s client set using the Amazon EKS cluster details
	clientSet, err := kubernetes.NewForConfig(&rest.Config{
		Host:        cluster.Endpoint,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: cluster.CA,
		},
	})
	if err != nil {
		fmt.Printf("Failed to create new k8s client")
		return "", nil, err
	}

	//CreateOrUpdateNodeGroupAuthConfigMap
	createOrUpdateAuthConfigMap, err := createOrUpdateNodeInstRoleAuthConfigMap(clientSet, nodeInstRoleARN)
	if err != nil {
		return "", nil, err
	}
	data = map[string]interface{}{
		"Created/Updated ConfigMap": createOrUpdateAuthConfigMap,
	}
	//Adding physcialResourceID as LogStream keeps changing
	physicalResourceID = "CreateOrUpdateConfigMapWithNodeInstanceRole"
	return
}

//CreateOrUpdateNodeInstRoleAuthConfigMap to modify aws-auth
func createOrUpdateNodeInstRoleAuthConfigMap(clientset *kubernetes.Clientset, nodeInstRoleARN string) (bool, error) {
	configMap := &corev1.ConfigMap{}
	client := clientset.CoreV1().ConfigMaps(configMapNamespace)
	create := false

	if existing, err := client.Get(configMapName, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) {
			create = true
		} else {
			return false, err
		}
	} else {
		*configMap = *existing
	}

	if create {
		configMap, err := NewAuthConfigMap(nodeInstRoleARN)
		if err != nil {
			return false, err
		}
		if _, err := client.Create(configMap); err != nil {
			return false, err
		}
		fmt.Printf("created auth ConfigMap")
		return false, nil
	}

	if err := UpdateAuthConfigMap(configMap, nodeInstRoleARN); err != nil {
		return false, err
	}
	if _, err := client.Update(configMap); err != nil {
		return false, err
	}
	fmt.Printf("updated auth ConfigMap")
	return true, nil
}

func makeMapRolesData() mapRolesData { return []map[string]interface{}{} }

func appendNodeInstRoleARNToAuthConfigMap(mapRoles *mapRolesData, nodeInstRoleARN string) {
	newEntry := map[string]interface{}{
		"rolearn":  nodeInstRoleARN,
		"username": "system:node:{{EC2PrivateDNSName}}",
		"groups": []string{
			"system:bootstrappers",
			"system:nodes",
		},
	}
	*mapRoles = append(*mapRoles, newEntry)
}

func newAuthConfigMap(mapRoles mapRolesData) (*corev1.ConfigMap, error) {
	mapRolesBytes, err := yaml.Marshal(mapRoles)
	if err != nil {
		return nil, err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: configMapNamespace,
		},
		Data: map[string]string{
			"mapRoles": string(mapRolesBytes),
		},
	}
	return cm, nil
}

// NewAuthConfigMap creates ConfigMap with a single nodegroup ARN
func NewAuthConfigMap(nodeInstRoleARN string) (*corev1.ConfigMap, error) {
	mapRoles := makeMapRolesData()
	appendNodeInstRoleARNToAuthConfigMap(&mapRoles, nodeInstRoleARN)
	return newAuthConfigMap(mapRoles)
}

func updateAuthConfigMap(cm *corev1.ConfigMap, mapRoles mapRolesData) error {
	mapRolesBytes, err := yaml.Marshal(mapRoles)
	if err != nil {
		return err
	}
	cm.Data["mapRoles"] = string(mapRolesBytes)
	return nil
}

// UpdateAuthConfigMap updates ConfigMap by appending a single nodegroup ARN
func UpdateAuthConfigMap(cm *corev1.ConfigMap, nodeInstRoleARN string) error {
	mapRoles := makeMapRolesData()
	if err := yaml.Unmarshal([]byte(cm.Data["mapRoles"]), &mapRoles); err != nil {
		return err
	}
	appendNodeInstRoleARNToAuthConfigMap(&mapRoles, nodeInstRoleARN)
	return updateAuthConfigMap(cm, mapRoles)
}

func main() {
	lambda.Start(cfn.LambdaWrap(Handler))
}
