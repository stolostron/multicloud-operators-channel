// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package utils

import (
	"bytes"
	"context"
	"io/ioutil"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/endpoints"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang/glog"
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
	// SecretMapKeySecretAccessKey is key of secretaccesskey in secret
	SecretMapKeySecretAccessKey = "SecretAccessKey"
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

	glog.Info("Preparing S3 settings")
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		glog.Error("Failed to load aws config. error: ", err)
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
		glog.Error("Failed to connect to s3 service")
		return err
	}

	h.Client.ForcePathStyle = true

	glog.Info("S3 configured ")

	return nil
}

// Create a bucket
func (h *AWSHandler) Create(bucket string) error {
	req := h.Client.CreateBucketRequest(&s3.CreateBucketInput{
		Bucket: &bucket,
	})

	_, err := req.Send(context.TODO())
	if err != nil {
		glog.Error("Failed to create bucket ", bucket, ". error: ", err)
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
		glog.Error("Failed to access bucket ", bucket, ". error: ", err)
		return err
	}

	return nil
}

// List all objects in bucket
func (h *AWSHandler) List(bucket string) ([]string, error) {

	glog.V(10).Info("List S3 Objects ", bucket)

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
		glog.Error("failed to list objects. error: ", err)
		return nil, err
	}

	glog.V(10).Info("List S3 Objects result ", keys)

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
		glog.Error("Failed to send Get request. error: ", err)
		return dplObj, err
	}

	generateName := resp.GetObjectOutput.Metadata[DployableMateGenerateNameKey]
	version := resp.GetObjectOutput.Metadata[DeployableMetaVersionKey]
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		glog.Error("Failed to parse Get request. error: ", err)
		return dplObj, err
	}

	dplObj.Name = name
	dplObj.GenerateName = generateName
	dplObj.Content = body
	dplObj.Version = version
	glog.V(10).Info("Get Success: \n", string(body))

	return dplObj, nil
}

// Put create new object
func (h *AWSHandler) Put(bucket string, dplObj DeployableObject) error {
	if dplObj.isEmpty() {
		glog.V(10).Infof("got an empty deployableObject to put to object store")
		return nil
	}

	req := h.Client.PutObjectRequest(&s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &dplObj.Name,
		Body:   bytes.NewReader(dplObj.Content),
	})

	req.HTTPRequest.Header.Set(DeployableGenerateNameMeta, dplObj.GenerateName)
	req.HTTPRequest.Header.Set(DeployableVersionMeta, dplObj.Version)

	resp, err := req.Send(context.Background())
	if err != nil {
		glog.Error("Failed to send Put request. error: ", err)
		return err
	}

	glog.V(10).Info("Put Success", resp)

	return nil
}

// Delete delete existing object
func (h *AWSHandler) Delete(bucket, name string) error {

	req := h.Client.DeleteObjectRequest(&s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &name,
	})

	resp, err := req.Send(context.Background())
	if err != nil {
		glog.Error("Failed to send Delete request. error: ", err)
		return err
	}

	glog.V(10).Info("Delete Success", resp)

	return nil
}