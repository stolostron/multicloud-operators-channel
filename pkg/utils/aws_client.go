// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"bytes"
	"context"
	"io/ioutil"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/endpoints"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ObjectStore interface
type ObjectStore interface {
	InitObjectStoreConnection(endpoint, accessKeyID, secretAccessKey string) error
	Exists(bucket string) error
	Create(bucket string) error
	List(bucket string) ([]string, error)
	Put(bucket string, dplObj DeployableObject) error
	Delete(bucket, name string) error
	Get(bucket, name string) (DeployableObject, error)
}

var _ ObjectStore = &AWSHandler{}

const (
	// SecretMapKeyAccessKeyID is key of accesskeyid in secret
	SecretMapKeyAccessKeyID = "AccessKeyID"
	USERNAME                = "username"
	// SecretMapKeySecretAccessKey is key of secretaccesskey in secret
	SecretMapKeySecretAccessKey = "SecretAccessKey"
	PASSWORD                    = "password"
	//metadata key for stroing the deployable generatename name
	DeployableGenerateNameMeta = "x-amz-meta-generatename"
	//Deployable generate name key within the meta map
	DployableMateGenerateNameKey = "Generatename"
	//metadata key for stroing the deployable generatename name
	DeployableVersionMeta = "x-amz-meta-deployableversion"
	//Deployable generate name key within the meta map
	DeployableMetaVersionKey = "Deployableversion"
)

// AWSHandler handles connections to aws
type AWSHandler struct {
	*s3.Client
}

// credentialProvider provides credetials for mcm hub deployable
type credentialProvider struct {
	AccessKeyID     string
	SecretAccessKey string
}

type DeployableObject struct {
	Name         string
	GenerateName string
	Version      string
	Content      []byte
}

func (d DeployableObject) isEmpty() bool {
	if d.Name == "" && d.GenerateName == "" && len(d.Content) == 0 {
		return true
	}

	return false
}

// Retrieve follow the Provider interface
func (p *credentialProvider) Retrieve() (aws.Credentials, error) {
	awscred := aws.Credentials{
		SecretAccessKey: p.SecretAccessKey,
		AccessKeyID:     p.AccessKeyID,
	}

	return awscred, nil
}

// InitObjectStoreConnection connect to object store
func (h *AWSHandler) InitObjectStoreConnection(endpoint, accessKeyID, secretAccessKey string) error {
	cfg, err := external.LoadDefaultAWSConfig()

	if err != nil {
		return err
	}
	// aws client report error without minio
	cfg.Region = "minio"

	defaultResolver := endpoints.NewDefaultResolver()
	s3CustResolverFn := func(service, region string) (aws.Endpoint, error) {
		if service == "s3" {
			return aws.Endpoint{
				URL: endpoint,
			}, nil
		}

		return defaultResolver.ResolveEndpoint(service, region)
	}

	cfg.EndpointResolver = aws.EndpointResolverFunc(s3CustResolverFn)
	cfg.Credentials = &credentialProvider{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}

	h.Client = s3.New(cfg)
	if h.Client == nil {
		return err
	}

	h.Client.ForcePathStyle = true

	return nil
}

// Create a bucket
func (h *AWSHandler) Create(bucket string) error {
	req := h.Client.CreateBucketRequest(&s3.CreateBucketInput{
		Bucket: &bucket,
	})

	_, err := req.Send(context.TODO())
	if err != nil {
		return err
	}

	return nil
}

// Exists Checks whether a bucket exists and is accessible
func (h *AWSHandler) Exists(bucket string) error {
	req := h.Client.HeadBucketRequest(&s3.HeadBucketInput{
		Bucket: &bucket,
	})

	_, err := req.Send(context.TODO())
	if err != nil {
		return err
	}

	return nil
}

// List all objects in bucket
func (h *AWSHandler) List(bucket string) ([]string, error) {
	req := h.Client.ListObjectsRequest(&s3.ListObjectsInput{Bucket: &bucket})
	p := s3.NewListObjectsPaginator(req)

	var keys []string

	for p.Next(context.TODO()) {
		page := p.CurrentPage()

		for _, obj := range page.Contents {
			keys = append(keys, *obj.Key)
		}
	}

	if err := p.Err(); err != nil {
		return nil, err
	}

	return keys, nil
}

// Get get existing object
func (h *AWSHandler) Get(bucket, name string) (DeployableObject, error) {
	dplObj := DeployableObject{}

	req := h.Client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &name,
	})

	resp, err := req.Send(context.Background())

	if err != nil {
		return dplObj, err
	}

	generateName := resp.GetObjectOutput.Metadata[DployableMateGenerateNameKey]
	version := resp.GetObjectOutput.Metadata[DeployableMetaVersionKey]
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return dplObj, err
	}

	dplObj.Name = name
	dplObj.GenerateName = generateName
	dplObj.Content = body
	dplObj.Version = version

	return dplObj, nil
}

// Put create new object
func (h *AWSHandler) Put(bucket string, dplObj DeployableObject) error {
	if dplObj.isEmpty() {
		return nil
	}

	req := h.Client.PutObjectRequest(&s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &dplObj.Name,
		Body:   bytes.NewReader(dplObj.Content),
	})

	req.HTTPRequest.Header.Set(DeployableGenerateNameMeta, dplObj.GenerateName)
	req.HTTPRequest.Header.Set(DeployableVersionMeta, dplObj.Version)

	_, err := req.Send(context.Background())
	if err != nil {
		return err
	}

	return nil
}

// Delete delete existing object
func (h *AWSHandler) Delete(bucket, name string) error {
	req := h.Client.DeleteObjectRequest(&s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &name,
	})

	_, err := req.Send(context.Background())
	if err != nil {
		return err
	}

	return nil
}
